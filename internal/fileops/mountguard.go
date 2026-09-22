package fileops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// mountRoots are the directories under which an operating system places
// mounted volumes. A configured library path under one of these is a mount,
// and if it is not currently mounted the directory still exists — empty, on
// the boot disk, and indistinguishable from the real thing to anything that
// only checks whether the path is readable.
var mountRoots = []string{"/Volumes/", "/mnt/", "/media/", "/run/media/"}

// UnderMountRoot reports whether a path lives where mounted volumes appear.
func UnderMountRoot(path string) bool {
	clean := filepath.Clean(path)
	for _, root := range mountRoots {
		if strings.HasPrefix(clean, root) {
			return true
		}
	}
	return false
}

// deviceIDFunc is the device lookup, replaceable in tests.
var deviceIDFunc = deviceID

// deviceID returns the filesystem device a path resides on.
func deviceID(path string) (uint64, error) {
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return 0, err
	}
	return uint64(st.Dev), nil
}

// ErrNotMounted reports a library path that should be a mounted volume but is
// currently sitting on the boot disk.
type ErrNotMounted struct {
	Path string
}

func (e *ErrNotMounted) Error() string {
	return fmt.Sprintf(
		"library path %s is on the boot disk, not a mounted volume; "+
			"the share is not mounted and writing here would fill the local disk "+
			"with files that look correctly placed. Mount it, or set "+
			"library.allow_unmounted=true if the library really is local",
		e.Path)
}

// VerifyMounted refuses a library path that should be a mount but is not.
//
// This exists because an unmounted share fails in the most expensive way
// available: the mount point remains as an ordinary empty directory, every
// readability check passes, and earworm's move path creates parent directories
// on demand. The result is gigabytes written to the boot volume, into a path
// that looks exactly right. Nothing downstream would notice.
//
// The test compares the path's device against the root filesystem's. Only
// paths under a conventional mount root are checked, so a library that
// genuinely lives on local disk is unaffected and needs no configuration.
//
// The nearest existing ancestor is used, because the leaf directory may not
// exist yet on a freshly mounted volume.
func VerifyMounted(path string) error {
	if path == "" || !UnderMountRoot(path) {
		return nil
	}

	probe := nearestExisting(path)
	if probe == "" {
		// Nothing along the path exists. The caller's availability probe
		// reports that; it is not this check's business.
		return nil
	}

	dev, err := deviceIDFunc(probe)
	if err != nil {
		return nil // unreadable is the availability probe's problem, not this one
	}
	rootDev, err := deviceIDFunc("/")
	if err != nil {
		return nil
	}

	if dev == rootDev {
		return &ErrNotMounted{Path: path}
	}
	return nil
}

// nearestExisting walks up from path to the first component that exists.
func nearestExisting(path string) string {
	p := filepath.Clean(path)
	for i := 0; i < 64; i++ {
		if _, err := os.Lstat(p); err == nil {
			return p
		}
		parent := filepath.Dir(p)
		if parent == p {
			return ""
		}
		p = parent
	}
	return ""
}

// destructiveVerbs are refused in a remount command.
//
// Mounting is recoverable; unmounting is not always. A forced unmount can cut
// short another process's write to the same share and leave a truncated file,
// and nothing about restoring a mount requires tearing one down first. The
// daemon is therefore allowed to mount and never to unmount, regardless of
// what the configuration asks for.
var destructiveVerbs = []string{
	"umount", "unmount", "eject", "diskutil erase", "diskutil reformat",
	"diskutil partitiondisk", "mkfs", "rm ", "rm-", "shutdown", "reboot",
}

// ErrUnsafeRemount reports a remount command that would do more than mount.
type ErrUnsafeRemount struct {
	Command string
	Verb    string
}

func (e *ErrUnsafeRemount) Error() string {
	return fmt.Sprintf(
		"library.remount_command contains %q, which earworm will not run unattended: "+
			"it can interrupt another process mid-write and truncate a file on the share. "+
			"Restrict the command to mounting, and unmount by hand if you need to",
		e.Verb)
}

// CheckRemountCommand rejects a remount command that does more than mount.
func CheckRemountCommand(command string) error {
	lower := strings.ToLower(command)
	for _, verb := range destructiveVerbs {
		if strings.Contains(lower, verb) {
			return &ErrUnsafeRemount{Command: command, Verb: strings.TrimSpace(verb)}
		}
	}
	return nil
}
