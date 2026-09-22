package fileops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnderMountRoot(t *testing.T) {
	for _, p := range []string{"/Volumes/media/Audible", "/mnt/nas/books", "/media/usb", "/run/media/x/y"} {
		assert.True(t, UnderMountRoot(p), p)
	}
	for _, p := range []string{"/Users/someone/Audiobooks", "/srv/library", "/opt/media", ""} {
		assert.False(t, UnderMountRoot(p), p)
	}
}

// The failure this exists to prevent: an unmounted share leaves an ordinary
// directory on the boot disk that passes every readability check, and writing
// a library into it fills the local volume with correctly-named files.
//
// When the share is absent the walk up reaches /Volumes itself, which lives on
// the boot disk, so the guard fires. That is the desired answer: naming the
// missing mount is more useful than a generic "no such file".
func TestVerifyMountedRejectsAbsentShare(t *testing.T) {
	if _, err := os.Stat("/Volumes"); err != nil {
		t.Skip("no /Volumes on this platform")
	}

	err := VerifyMounted("/Volumes/earworm-test-definitely-not-mounted/library")
	require.Error(t, err)

	var notMounted *ErrNotMounted
	assert.ErrorAs(t, err, &notMounted)
	assert.Contains(t, err.Error(), "not a mounted volume")
	assert.Contains(t, err.Error(), "fill the local disk",
		"the consequence is the part worth stating")
}

// A directory that exists on the boot disk under a mount root is the exact
// shape of an unmounted share, and is the case this guard exists for.
//
// The mount-root list is overridden rather than writing under /Volumes, which
// needs privileges — a skipped test for the central case is no coverage at all.
func TestVerifyMountedRejectsRealBootDiskDirectoryUnderMountRoot(t *testing.T) {
	dir := t.TempDir()
	library := filepath.Join(dir, "media", "Audible")
	require.NoError(t, os.MkdirAll(library, 0o755))

	orig := mountRoots
	mountRoots = []string{dir + "/"}
	t.Cleanup(func() { mountRoots = orig })

	err := VerifyMounted(library)
	require.Error(t, err, "a real boot-disk directory under a mount root is an unmounted share")

	var notMounted *ErrNotMounted
	assert.ErrorAs(t, err, &notMounted)
	assert.Contains(t, err.Error(), "fill the local disk")
}

// The same path is fine once it is genuinely a different filesystem, which is
// what a real mount provides.
func TestVerifyMountedAcceptsDifferentDevice(t *testing.T) {
	dir := t.TempDir()
	orig := mountRoots
	mountRoots = []string{dir + "/"}
	t.Cleanup(func() { mountRoots = orig })

	// Substitute a device lookup that reports a distinct filesystem, standing
	// in for a mounted share.
	origDev := deviceIDFunc
	deviceIDFunc = func(path string) (uint64, error) {
		if path == "/" {
			return 1, nil
		}
		return 2, nil
	}
	t.Cleanup(func() { deviceIDFunc = origDev })

	assert.NoError(t, VerifyMounted(filepath.Join(dir, "media")))
}

func TestVerifyMountedAllowsGenuineMount(t *testing.T) {
	// Any real mount whose device differs from root satisfies the check.
	if _, err := os.Stat("/Volumes"); err != nil {
		t.Skip("no /Volumes on this platform")
	}
	entries, err := os.ReadDir("/Volumes")
	require.NoError(t, err)

	rootDev, err := deviceID("/")
	require.NoError(t, err)

	for _, e := range entries {
		p := filepath.Join("/Volumes", e.Name())
		dev, err := deviceID(p)
		if err != nil || dev == rootDev {
			continue
		}
		assert.NoError(t, VerifyMounted(p), "a genuinely mounted volume must pass")
		return
	}
	t.Skip("no mounted volume available to verify against")
}

// A library that genuinely lives on local disk must not be second-guessed.
func TestVerifyMountedIgnoresPathsOutsideMountRoots(t *testing.T) {
	assert.NoError(t, VerifyMounted(t.TempDir()))
	assert.NoError(t, VerifyMounted("/Users/someone/Audiobooks"))
	assert.NoError(t, VerifyMounted(""))
}

func TestNearestExistingWalksUp(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b", "c")
	assert.Equal(t, dir, nearestExisting(deep))
	assert.Equal(t, dir, nearestExisting(dir))
}

// Mounting is recoverable; a forced unmount can cut short another process's
// write to the same share.
func TestCheckRemountCommandRejectsDestructiveVerbs(t *testing.T) {
	unsafe := []string{
		"diskutil unmount force /Volumes/media",
		"umount /Volumes/media && mount -t smbfs //host/share /Volumes/media",
		"hdiutil eject /Volumes/media",
		"diskutil eraseDisk JHFS+ media disk2",
		"mkfs.ext4 /dev/sdb1",
		"rm -rf /Volumes/media && mount ...",
	}
	for _, c := range unsafe {
		err := CheckRemountCommand(c)
		require.Error(t, err, "should refuse: %s", c)
		assert.Contains(t, err.Error(), "will not run unattended")
	}
}

func TestCheckRemountCommandAllowsMounting(t *testing.T) {
	safe := []string{
		"open 'smb://user@host/share'",
		"mount -t smbfs //user@host/share /Volumes/media",
		"mount_smbfs //user@host/share /Volumes/media",
		"/usr/local/bin/mount-nas.sh",
	}
	for _, c := range safe {
		assert.NoError(t, CheckRemountCommand(c), "should allow: %s", c)
	}
}

func TestCheckRemountCommandIsCaseInsensitive(t *testing.T) {
	assert.Error(t, CheckRemountCommand("DISKUTIL UNMOUNT /Volumes/media"))
}
