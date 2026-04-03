# Domain Pitfalls

**Domain:** Audiobook library management CLI (Go wrapper around audible-cli, NAS-targeted)
**Researched:** 2026-04-03

## Critical Pitfalls

Mistakes that cause rewrites, data loss, or project-killing issues.

### Pitfall 1: SQLite Database on Network Filesystem

**What goes wrong:** Storing the SQLite database on the NAS mount (alongside the audiobook files) leads to silent corruption. SQLite relies on POSIX `fcntl()` advisory locking which is unreliable over NFS and SMB. WAL mode's shared memory mappings (`-shm` file) do not work over network filesystems at all. Transactions can appear to succeed while the remote database silently corrupts. Data can arrive out of order, and incomplete writes may not be detected.

**Why it happens:** It feels natural to co-locate the database with the library it indexes. The NAS mount "looks like" a local filesystem. Corruption is intermittent and may not appear until weeks of use, making it hard to reproduce.

**Consequences:** Library index corruption, lost download state, duplicate downloads, or complete data loss requiring a full re-index. Recovery is manual and painful. SQLite's official documentation explicitly warns: "avoid remote SQLite databases unless reliability is unimportant."

**Prevention:**
- Store SQLite database on the LOCAL filesystem always (e.g., `~/.config/earworm/earworm.db` or XDG data directory)
- Store only audiobook files (M4A, covers, metadata JSON) on the NAS mount
- Document this architectural decision prominently so users do not attempt to relocate the DB
- Consider adding a startup check that detects if the DB path is on a network mount and warns/refuses

**Detection:** Database corruption errors ("database disk image is malformed"), unexplained duplicate entries, download state resets, or intermittent "database is locked" errors.

**Phase relevance:** Phase 1 (database design). Getting this wrong early means a rewrite.

**Confidence:** HIGH -- SQLite official docs explicitly document this. Multiple real-world projects (Sonarr, GoToSocial, TrueNAS community) confirm the issue.

**Sources:**
- [SQLite Over a Network, Caveats and Considerations](https://sqlite.org/useovernet.html)
- [Sonarr SQLite on Network Share Issue #1886](https://github.com/Sonarr/Sonarr/issues/1886)
- [GoToSocial SQLite on Networked Storage](https://docs.gotosocial.org/en/latest/advanced/sqlite-networked-storage/)

---

### Pitfall 2: Audible Auth Token Mismanagement

**What goes wrong:** Audible access tokens expire after 60 minutes. The refresh token obtained during device registration is the only way to get new access tokens. If the refresh token is lost, corrupted, or invalidated (e.g., by deregistering the device), the user must re-authenticate from scratch -- which involves an interactive browser-based login flow that cannot be automated.

**Why it happens:** Developers treat auth as a "set it and forget it" step. They don't handle token refresh proactively, don't protect the credential file from corruption during writes, or accidentally trigger deregistration. Every device registration appears on the user's Amazon device list, so repeated registrations look suspicious and create device slot clutter.

**Consequences:** Users get locked out of downloads with no automated recovery path. If the tool registers multiple devices (e.g., due to bugs or retries), it fills up the user's Audible device slots. Worst case: Amazon flags the account for suspicious activity.

**Prevention:**
- Treat the audible-cli credential/auth file as precious state -- never write to it except through audible-cli itself
- Register the device ONCE and persist the credential file reliably (local filesystem, not NAS)
- Implement proactive token refresh before the 60-minute expiry window, not reactively on 401 errors
- Never call `deregister_device` automatically -- make it an explicit user action with warnings
- Add a pre-flight check before downloads: "is the token valid? can we refresh it?" with clear error messages if not
- Store auth files with restrictive permissions (0600)

**Detection:** HTTP 401 errors during API calls, "token expired" messages from audible-cli, users reporting they need to re-authenticate frequently.

**Phase relevance:** Phase 1-2 (auth integration). Must be correct before any download logic.

**Confidence:** MEDIUM -- Based on audible library docs and community issues. Token expiry timing confirmed in official docs. Specific ban thresholds are undocumented.

**Sources:**
- [Audible Authentication Documentation](https://audible.readthedocs.io/en/latest/auth/authentication.html)
- [audible-cli Issue #42 - Token stdout for automation](https://github.com/mkb79/audible-cli/issues/42)
- [Audible Device Registration Docs](https://audible.readthedocs.io/en/latest/auth/register.html)

---

### Pitfall 3: Audible Rate Limiting and Account Bans

**What goes wrong:** Downloading an entire library (potentially hundreds of books) in rapid succession triggers Audible's undocumented rate limits. The API is private and undocumented -- there are no published rate limit thresholds. Aggressive downloading can result in temporary or permanent account restrictions.

**Why it happens:** When a user first sets up the tool, they want to download their entire library. The naive approach is to loop through all books and download sequentially as fast as possible. Audible's rate limiting behavior is not publicly documented, so developers can't implement precise throttling.

**Consequences:** Temporary download blocks (hours to days), account flagging, or in extreme cases permanent account restrictions. Once flagged, there's no automated recovery -- the user must contact Audible support.

**Prevention:**
- Implement conservative rate limiting by default: 1 book download at a time with configurable delays between downloads (start with 30-60 seconds between books)
- Add exponential backoff on ANY HTTP error (429, 5xx, connection reset)
- Make rate limiting configurable but set safe defaults that users must explicitly override
- Add a `--slow` / `--fast` flag or `downloads-per-hour` config with a low default (e.g., 10-20 books/hour)
- Log all download timestamps so patterns can be analyzed if issues arise
- On first run with a large library, prompt the user about expected download time rather than silently hammering the API
- Implement a circuit breaker: after N consecutive failures, pause all downloads for a cooling period and notify the user

**Detection:** HTTP 429 responses, connection resets, increasing download failures, audible-cli returning error codes without clear messages.

**Phase relevance:** Phase 2-3 (download logic). Must be built into the download pipeline from the start, not bolted on later.

**Confidence:** LOW-MEDIUM -- Rate limit specifics are undocumented. The risk is real but thresholds are unknown. Conservative defaults are the only safe approach.

**Sources:**
- [audible-cli GitHub Repository](https://github.com/mkb79/audible-cli)
- [OpenAudible Documentation](https://openaudible.org/documentation) (mentions rate limit handling)

---

### Pitfall 4: Go-to-Python Subprocess Lifecycle Mismanagement

**What goes wrong:** Go's `exec.Command` wrapping Python's audible-cli creates multiple failure modes: zombie processes when `Wait()` is not called, orphaned Python processes that survive Go crashes, hanging downloads when Python blocks on I/O but Go's context is cancelled, and resource leaks from unclosed pipes.

**Why it happens:** Go and Python have fundamentally different process models. Go's goroutine-based concurrency does not automatically clean up child processes. Python subprocesses can buffer output causing pipe deadlocks. If Go crashes or is killed (SIGKILL), child Python processes become orphans. Context cancellation in Go sends signals but Python may not handle them gracefully.

**Consequences:** Zombie processes accumulating over time, leaked file descriptors, partial downloads with no cleanup, system resource exhaustion on long-running daemon-style usage, and hung earworm processes that require manual killing.

**Prevention:**
- ALWAYS call `cmd.Wait()` after `cmd.Start()` -- use `defer` patterns
- Set `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}` to create process groups, enabling cleanup of the entire process tree
- Use `context.WithTimeout` for ALL subprocess calls with generous but finite timeouts (audiobook downloads can take 10+ minutes for long books)
- Implement signal forwarding: trap SIGINT/SIGTERM in Go, forward to child process group, wait for clean exit, then exit Go
- Capture both stdout and stderr via pipes, read them concurrently (not sequentially) to avoid pipe buffer deadlocks
- Use `cmd.Process.Kill()` followed by `cmd.Wait()` for forced cleanup -- Kill alone is not sufficient
- Add a process watchdog: if a download subprocess has not produced output for N minutes, kill and retry
- On startup, check for and clean up stale PID files / lock files from previous crashed runs

**Detection:** `ps aux | grep audible` showing orphaned processes, increasing memory/FD usage over time, downloads that "hang" indefinitely, earworm refusing to start due to stale lock files.

**Phase relevance:** Phase 1-2 (subprocess wrapper design). The abstraction layer around audible-cli must be robust before building download orchestration on top of it.

**Confidence:** HIGH -- Well-documented Go issue. Multiple Go stdlib issues confirm the behavior.

**Sources:**
- [Go Exec zombies and orphaned processes](https://segmentfault.com/a/1190000041466423/en)
- [os/exec Issue #52580 - Wait() documentation](https://github.com/golang/go/issues/52580)
- [Clean exit of Go exec.Command](https://medium.com/@ganeshmaharaj/clean-exit-of-golangs-exec-command-897832ac3fa5)

---

## Moderate Pitfalls

### Pitfall 5: Large File Downloads Over Network Mounts

**What goes wrong:** Audiobook files are large (50MB-500MB each). Writing them directly to a NAS mount over SMB/NFS introduces failure modes that don't exist with local storage: network timeouts mid-write, SMB session disconnects on long transfers, partial files from interrupted writes, and filesystem caching behavior that makes progress tracking unreliable.

**Prevention:**
- Download to a LOCAL temporary directory first, then move/copy to NAS mount
- Use atomic file operations: write to `filename.tmp`, then rename to `filename` on completion
- Implement resumable downloads if audible-cli supports it (check for partial file + byte range requests)
- Set generous but finite I/O timeouts on file copy operations
- Verify file integrity after copy (file size at minimum, checksum if available)
- Track download state in the local SQLite DB: NOT_STARTED -> DOWNLOADING -> DOWNLOADED -> COPYING -> COMPLETE
- If NAS mount disappears mid-copy, detect it (check mount point exists) and pause rather than writing to the mount point directory on the local filesystem

**Detection:** Truncated files on NAS, `.tmp` files that never get renamed, downloads that appear complete in the DB but files are missing or corrupt, "stale NFS file handle" errors.

**Phase relevance:** Phase 2-3 (download and file organization). The download-then-copy pattern should be established early.

**Confidence:** HIGH -- Well-known issue with network filesystems. TrueNAS community threads document this extensively.

**Sources:**
- [TrueNAS - Huge file copy fails](https://www.truenas.com/community/threads/huge-file-copy-fails-every-time.94695/)
- [Linux Mint - SMB share file transfer crashing](https://forums.linuxmint.com/viewtopic.php?t=417575)

---

### Pitfall 6: Libation File Structure Compatibility Drift

**What goes wrong:** Earworm replicates Libation's file organization by observation, not by sharing code. Libation's naming templates are user-configurable and the defaults can change between versions. If Earworm hardcodes a specific structure, it breaks when Libation updates its defaults or when users have customized templates. Worse, Libation identifies files by ASIN via a `FilePathCache` -- it searches subdirectories for files matching product IDs, not by exact path matching.

**Prevention:**
- Document which Libation version's default structure is being replicated and keep it as a configurable template
- Include the Audible ASIN in directory or filenames (this is how Libation locates files)
- Implement Earworm's own template system for file naming, with a Libation-compatible default
- Test compatibility by pointing Libation at an Earworm-organized library and verifying it recognizes all books
- Keep cover art, metadata sidecar files, and audio files co-located as Libation expects
- Do NOT assume `.m4a` extension -- Libation uses `.m4b` for all non-MP3 files regardless of codec

**Detection:** Libation failing to recognize Earworm-organized books, duplicate entries in Libation, files appearing as "not downloaded" in Libation despite being present.

**Phase relevance:** Phase 2 (file organization). Should be validated early with manual testing against an actual Libation installation.

**Confidence:** MEDIUM -- Libation's docs describe the template system and ASIN lookup, but specific defaults are not publicly documented in a stable way.

**Sources:**
- [Libation Naming Templates](https://getlibation.com/docs/features/naming-templates)
- [Libation File Management - DeepWiki](https://deepwiki.com/rmcrackan/Libation/3.3-file-management)
- [Libation Audio Formats](https://getlibation.com/docs/features/audio-file-formats)

---

### Pitfall 7: M4A/M4B Metadata Handling Complexity

**What goes wrong:** M4A/M4B files use the MP4 container format with complex metadata atoms. Chapter information, cover art, and audiobook-specific tags (narrator, series, series number) use non-standard or Apple-specific atoms that mainstream Go libraries may not fully support. The `dhowden/tag` Go library supports reading MP4 metadata but may not support writing all audiobook-specific fields.

**Prevention:**
- Do NOT attempt to write M4A metadata from Go directly in v1 -- let audible-cli handle the download with metadata intact
- For any metadata operations, shell out to `ffprobe`/`ffmpeg` rather than implementing MP4 atom manipulation in Go
- Read metadata (for indexing) using `dhowden/tag` or `ffprobe` -- validate that chapter markers, cover art, and series info are accessible
- Store extracted metadata in SQLite for querying without re-reading files
- If Audiobookshelf handles its own metadata scanning, avoid duplicating that work -- just organize files correctly and trigger a scan

**Detection:** Missing chapter markers in Audiobookshelf, incorrect series ordering, missing cover art, metadata showing as "Unknown" in playback apps.

**Phase relevance:** Phase 2 (library scanning/indexing). Read-only metadata operations first, defer any write operations.

**Confidence:** MEDIUM -- Go MP4 metadata ecosystem is limited. `dhowden/tag` is the best option but focused on reading, not writing.

**Sources:**
- [dhowden/tag - Go MP4 metadata](https://github.com/dhowden/tag)
- [m4b package for Go](https://pkg.go.dev/github.com/achwo/narr/m4b)
- [audiobook-split-ffmpeg-go](https://github.com/MawKKe/audiobook-split-ffmpeg-go)

---

### Pitfall 8: GPL License Contamination from Libation

**What goes wrong:** Libation is GPL-licensed. Earworm is MIT/Apache. If any Libation source code is referenced, copied, or adapted during development -- even indirectly (e.g., reading Libation source to understand file structure, then implementing the same logic) -- it creates a GPL contamination risk. The legal line between "observing behavior" and "copying implementation" is blurry.

**Prevention:**
- NEVER read Libation source code during development. Treat this as a clean-room constraint.
- Document the file structure based only on observing actual file output (examining a Libation-organized library on disk) and Libation's public user documentation (naming templates docs, user guides)
- The Libation naming template documentation and user-facing settings are not GPL-protected -- they describe user-configurable behavior, not implementation
- Keep a clear paper trail: document WHERE each piece of compatibility knowledge came from (user docs URL, file system observation, etc.)
- If uncertain about a specific implementation detail, derive it from Audiobookshelf's expectations instead (since Audiobookshelf is the actual target media server)
- Copyright protects expression, not ideas. A file structure convention (Author/Title/file.m4b) is a functional arrangement, not copyrightable expression. But specific template parsing logic or unique algorithmic choices could be.

**Detection:** Code review finding patterns identical to Libation source, git history showing Libation repo was cloned/browsed around the time of implementation.

**Phase relevance:** All phases. Establish the clean-room discipline from day one and document it.

**Confidence:** MEDIUM -- Legal risk assessment based on general GPL clean-room principles, not legal advice.

**Sources:**
- [Asahi Linux Copyright & Reverse Engineering](https://asahilinux.org/copyright/)
- [Clean Room Reverse Engineering - RetroReversing](https://www.retroreversing.com/clean-room-reversing)
- [Libation GitHub](https://github.com/rmcrackan/Libation) (GPL-licensed)

---

## Minor Pitfalls

### Pitfall 9: Python Environment Detection Fragility

**What goes wrong:** audible-cli requires Python 3. The Go binary needs to find and invoke the correct Python installation. Users may have Python 2 as default `python`, Python 3 as `python3`, or use pyenv/conda/venv. The tool might find the wrong Python, or audible-cli might not be installed in the detected Python's environment.

**Prevention:**
- Check for `audible` command directly (not `python -m audible_cli`) as the primary detection method
- Fall back to `python3 -m audible` then `python -m audible`
- On first run, validate: (1) audible-cli is found, (2) it's the expected minimum version, (3) it can execute a basic command (e.g., `audible --version`)
- Allow explicit configuration: `EARWORM_AUDIBLE_PATH=/path/to/audible` environment variable
- Document installation requirements clearly, including pipx as the recommended install method

**Detection:** "command not found" errors, version mismatch errors, import errors from audible-cli.

**Phase relevance:** Phase 1 (initial setup and dependency detection).

**Confidence:** HIGH -- Standard cross-platform Python distribution challenge.

---

### Pitfall 10: Audiobookshelf API Integration Assumptions

**What goes wrong:** Assuming Audiobookshelf's library scan API is synchronous or fast. Library scans for large collections can take minutes. The API endpoint may change between Audiobookshelf versions. The scan might not pick up files immediately due to filesystem caching on the NAS.

**Prevention:**
- Trigger scan, then poll for completion (or fire-and-forget with a log message)
- Version-pin the Audiobookshelf API endpoint and document the minimum supported version
- Add a configurable delay between file copy completion and scan trigger (to allow NFS/SMB cache flush)
- Make Audiobookshelf integration optional -- not all users will run it
- Handle Audiobookshelf being unreachable gracefully (log warning, don't fail the download)

**Detection:** Newly downloaded books not appearing in Audiobookshelf, scan trigger returning errors, books appearing with missing metadata.

**Phase relevance:** Phase 3-4 (integration phase). Lower priority than core download functionality.

**Confidence:** MEDIUM -- Based on general REST API integration patterns and Audiobookshelf documentation.

---

### Pitfall 11: Download State Recovery After Crashes

**What goes wrong:** If earworm crashes mid-download, the state must be recoverable. Common mistakes: marking a download as "complete" before the file is fully written, not tracking which step of a multi-step process (download -> decrypt -> move -> verify) was completed, and leaving temporary files that block retries.

**Prevention:**
- Use a state machine for each download: QUEUED -> DOWNLOADING -> DOWNLOADED -> ORGANIZING -> COMPLETE (and FAILED)
- Only advance state AFTER the operation is confirmed (file exists, size matches, etc.)
- On startup, scan for and clean up incomplete states: DOWNLOADING -> reset to QUEUED, ORGANIZING -> check if file exists
- Use a dedicated temp directory that gets cleaned on startup
- Implement an idempotent retry: re-running a failed download from any state should work without manual intervention

**Detection:** Stale temp files, downloads stuck in intermediate states, `earworm status` showing inconsistencies between DB state and actual files on disk.

**Phase relevance:** Phase 2 (download pipeline). Build state recovery from the start.

**Confidence:** HIGH -- Standard distributed systems / download manager pattern.

---

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| Database schema design | SQLite on NAS mount | Store DB locally, never on network filesystem |
| Auth integration | Token expiry, device slot pollution | Register once, persist creds locally, refresh proactively |
| Subprocess wrapper | Zombie/orphan processes, pipe deadlocks | Process groups, concurrent pipe reads, context timeouts |
| Download pipeline | Rate limiting, large file network writes | Conservative throttling, download-to-local-then-copy pattern |
| File organization | Libation compatibility drift, GPL contamination | Template system, clean-room discipline, ASIN in paths |
| Metadata handling | M4A atom complexity | Read-only via ffprobe, defer write operations |
| External integrations | Audiobookshelf API assumptions | Optional, fire-and-forget, version-pinned |
| First-run experience | Python environment detection | Multiple detection strategies, explicit config override |

## Sources

- [SQLite Over a Network](https://sqlite.org/useovernet.html)
- [Audible Authentication Docs](https://audible.readthedocs.io/en/latest/auth/authentication.html)
- [Audible Device Registration](https://audible.readthedocs.io/en/latest/auth/register.html)
- [audible-cli GitHub](https://github.com/mkb79/audible-cli)
- [Libation Naming Templates](https://getlibation.com/docs/features/naming-templates)
- [Libation File Management - DeepWiki](https://deepwiki.com/rmcrackan/Libation/3.3-file-management)
- [dhowden/tag Go library](https://github.com/dhowden/tag)
- [Go exec.Command zombie processes](https://segmentfault.com/a/1190000041466423/en)
- [Asahi Linux Clean Room RE](https://asahilinux.org/copyright/)
