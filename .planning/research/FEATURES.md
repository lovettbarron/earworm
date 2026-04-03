# Feature Research

**Domain:** CLI audiobook library management (Audible ecosystem)
**Researched:** 2026-04-03
**Confidence:** HIGH

## Feature Landscape

### Table Stakes (Users Expect These)

Features users assume exist. Missing these = product feels incomplete.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Audible authentication via audible-cli | Cannot do anything without auth; every tool has this | LOW | Wrap `audible quickstart` and `audible manage auth-file` subprocesses. Auth files are stored by audible-cli itself. |
| Library listing from Audible account | Must know what books exist to download them; audible-cli, Libation, OpenAudible all do this | LOW | `audible library list` and `audible library export --format json` provide full library metadata. |
| Download audiobooks from Audible | Core purpose of tool. Libation, OpenAudible, audible-cli all do this. | MEDIUM | Wrap `audible download` with ASIN targeting. audible-cli handles AAXC format + resume of partial downloads natively. |
| Local library scanning | Must detect what is already downloaded vs. what is missing. Libation scans by ASIN in subdirectories. | MEDIUM | Walk directory tree, match files to known ASINs. Must handle Libation's convention of embedding ASIN in folder names. |
| SQLite state tracking | Need persistent record of library state, download status, metadata. Standard for CLI tools (audible-cli uses it, Libation uses LibationContext.db). | MEDIUM | Schema: books table (ASIN, title, author, series, narrator, download_status, file_path, timestamps). |
| Download status tracking | Users need to know what downloaded, what failed, what is pending. All tools show this. | LOW | Track per-book: pending, downloading, completed, failed, skipped. Store in SQLite. |
| Cover art download | All tools download cover art. Audiobookshelf and media players expect it alongside audio files. | LOW | audible-cli supports `--cover` flag on download. Save as cover.jpg or folder.jpg in book directory. |
| Metadata preservation | Title, author, narrator, series, duration, publish date. All tools preserve this. Audiobookshelf parses it from folder structure + ID3 tags. | MEDIUM | Store in SQLite from audible-cli library export JSON. audible-cli embeds ID3 tags during download. |
| Configurable output directory | Users store libraries on NAS mounts, external drives, etc. All tools support custom output paths. | LOW | CLI flag + config file. Default to current directory or configured library root. |
| Progress reporting during downloads | Downloads take hours for large libraries. Users need visibility. Libation shows progress; its crashes during long runs are a top complaint. | MEDIUM | Parse audible-cli stdout for progress. Report per-book and overall progress. |
| Rate limiting / backoff | Audible will throttle or ban aggressive downloaders. Essential for long-running batch downloads. | MEDIUM | Configurable delay between downloads. Exponential backoff on HTTP errors. Respect audible-cli's own rate handling. |

### Differentiators (Competitive Advantage)

Features that set Earworm apart from Libation, OpenAudible, and raw audible-cli usage.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Fault-tolerant batch downloads | Libation crashes mid-download on large libraries (well-documented: crashes at 2hrs into 6hr sessions, crashes after green light, database corruption). Earworm's core value is surviving interruptions gracefully. | HIGH | Track each book independently. If process dies, resume from last incomplete book on restart. audible-cli already handles partial file resume for individual books. Earworm adds queue-level resilience on top. |
| Automatic new book detection | Check Audible account for books not yet in local library. Libation has this but it is coupled to its unreliable GUI. No CLI tool does this well. | MEDIUM | Diff audible-cli library export against SQLite state. Report new books, optionally auto-download. |
| Audiobookshelf scan trigger | After downloads complete, automatically notify Audiobookshelf to pick up new content. No existing tool integrates this. | LOW | Single POST to `/api/libraries/<ID>/scan` with Bearer token. Config stores API URL + token + library ID. |
| Libation-compatible folder structure | Existing Libation users can switch without reorganizing their library. Audiobookshelf already configured to read this structure. | MEDIUM | Replicate Libation's `Author/Title [ASIN]` convention. Include cover art and metadata files in same locations Libation uses. |
| Headless/daemon mode | Run on a server or NAS host, polling for new books periodically. Libation and OpenAudible are GUI apps. audible-cli is manual. | MEDIUM | Periodic polling loop with configurable interval. Log-based output instead of interactive. Suitable for cron or systemd. |
| Goodreads sync integration | Mark finished audiobooks on Goodreads. No other download tool integrates this. | MEDIUM | Wrap `good-audible-story-sync` (Ruby gem) or implement similar logic. Secondary priority -- the gem is macOS-focused and uses Selenium-style auth. |
| Chapter file download | audible-cli supports downloading chapter metadata. Useful for Audiobookshelf chapter navigation. | LOW | `audible download --chapter` flag. Store .json chapter file alongside audio. |
| Dry-run mode | Preview what would be downloaded without actually downloading. Useful for large libraries. | LOW | List new/missing books, show estimated download size, exit without downloading. |
| Machine-readable output (JSON) | Enable scripting and pipeline integration. Raw audible-cli output is not structured. | LOW | `--output json` flag on list/status/download commands. Enables integration with jq, scripts, dashboards. |

### Anti-Features (Commonly Requested, Often Problematic)

Features that seem good but create problems for Earworm specifically.

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| AAX/AAXC decryption within Earworm | "One tool to do everything" | License contamination risk (Libation is GPL), massive complexity, duplicates audible-cli's job. audible-cli already handles decryption. | Delegate entirely to audible-cli. Earworm is an orchestrator, not a codec tool. |
| Audio playback | "Preview my books" | Earworm is a library manager, not a player. Audiobookshelf handles playback. Adding playback means audio library dependencies, UI complexity. | Point users to Audiobookshelf for playback. |
| Multi-format conversion (MP3, M4B, FLAC) | "I want MP3s" | Format conversion is a deep rabbit hole (ffmpeg dependency, quality settings, chapter handling). OpenAudible charges for this. | v1: M4A only (audible-cli default output). Document how to use ffmpeg externally if needed. |
| GUI / TUI with rich widgets | "Make it look nice" | Massive scope increase. TUI frameworks (bubbletea, etc.) add complexity. The value is reliability, not prettiness. | Clean CLI output with good formatting. JSON output for custom dashboards. |
| Direct Audible API implementation | "Remove Python dependency" | audible-cli has years of auth flow handling, DRM negotiation, regional quirks. Reimplementing is months of work with ongoing maintenance as Audible changes. | Wrap audible-cli. Accept Python as a runtime dependency. Document installation. |
| Multi-service support (Libro.fm, Chirp, etc.) | "Support more than Audible" | Each service has different APIs, auth, DRM. Scope explosion. | v1: Audible only. Architecture should not preclude future services, but do not build for them now. |
| Automatic library reorganization / renaming | "Fix my existing messy library" | Destructive operation on user files. Risk of data loss. Complex edge cases (duplicates, partial names, special characters). | Scan-only for existing libraries. Only organize files that Earworm downloads itself. |
| Real-time webhook notifications | "Notify me on Slack/Discord when download finishes" | Scope creep. Webhook infrastructure, retry logic, auth for each service. | Log output + exit codes. Users can wrap with their own notification scripts. |

## Feature Dependencies

```
[Audible Authentication]
    +--requires--> [audible-cli installed and configured]
    |
    +--enables--> [Library Listing from Audible]
                      +--enables--> [New Book Detection]
                      +--enables--> [Download Audiobooks]
                                        +--requires--> [Rate Limiting]
                                        +--requires--> [Download Status Tracking]
                                        +--enables--> [Fault-Tolerant Batch Downloads]
                                        +--enables--> [Audiobookshelf Scan Trigger]
                                        +--enables--> [Goodreads Sync]

[Local Library Scanning]
    +--requires--> [SQLite State Tracking]
    +--requires--> [Libation-Compatible Folder Structure]
    +--enables--> [New Book Detection] (by diffing local vs remote)

[SQLite State Tracking]
    +--enables--> [Download Status Tracking]
    +--enables--> [Local Library Scanning]
    +--enables--> [New Book Detection]

[Headless/Daemon Mode]
    +--requires--> [New Book Detection]
    +--requires--> [Fault-Tolerant Batch Downloads]
    +--requires--> [Audiobookshelf Scan Trigger]
```

### Dependency Notes

- **Download requires Auth**: Cannot download without valid audible-cli auth session. Auth must be the first thing configured.
- **New Book Detection requires both Library Listing and Local Scan**: Must know what Audible has AND what is already local to compute the diff.
- **Fault-Tolerant Batch Downloads requires Status Tracking**: Cannot resume a batch without persistent record of what succeeded/failed.
- **Headless Mode requires everything else**: It is the capstone feature that automates the full pipeline. Build last.
- **Audiobookshelf Scan Trigger is independent**: Simple HTTP call. Can be added at any point after downloads work.

## MVP Definition

### Launch With (v1)

Minimum viable product -- what is needed to replace manual audible-cli usage.

- [ ] Audible authentication (wrap audible-cli auth) -- cannot function without it
- [ ] Library listing from Audible account -- must see what is available
- [ ] Local library scanning with ASIN matching -- must know what is already downloaded
- [ ] SQLite state tracking -- persistent knowledge of library state
- [ ] Download audiobooks with status tracking -- the core action
- [ ] Rate limiting and backoff -- protect against Audible throttling
- [ ] Fault-tolerant batch downloads (queue-level resume) -- the core differentiator
- [ ] Libation-compatible folder organization -- compatibility with existing libraries
- [ ] Cover art download -- expected by Audiobookshelf
- [ ] Progress reporting -- visibility during long operations
- [ ] New book detection (diff remote vs local) -- know what to download

### Add After Validation (v1.x)

Features to add once core downloading is stable.

- [ ] Audiobookshelf scan trigger -- add once download pipeline is reliable
- [ ] Chapter file download -- add once basic downloads work
- [ ] Dry-run mode -- add once new book detection works
- [ ] JSON output mode -- add when scripting integrations are needed
- [ ] Configurable naming templates -- add if users need non-default folder structures

### Future Consideration (v2+)

Features to defer until core is battle-tested.

- [ ] Headless/daemon mode with polling -- requires all v1 features to be rock-solid
- [ ] Goodreads sync -- secondary workflow, depends on fragile Ruby gem or custom implementation
- [ ] Multi-region Audible support -- audible-cli supports it, but adds testing complexity
- [ ] Notification hooks (post-download scripts) -- simple but scope creep; add when users ask

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Audible authentication (wrap) | HIGH | LOW | P1 |
| Library listing from Audible | HIGH | LOW | P1 |
| Local library scanning | HIGH | MEDIUM | P1 |
| SQLite state tracking | HIGH | MEDIUM | P1 |
| Download audiobooks | HIGH | MEDIUM | P1 |
| Rate limiting / backoff | HIGH | MEDIUM | P1 |
| Fault-tolerant batch downloads | HIGH | HIGH | P1 |
| Libation-compatible folder structure | HIGH | MEDIUM | P1 |
| Cover art download | MEDIUM | LOW | P1 |
| Progress reporting | MEDIUM | MEDIUM | P1 |
| New book detection | HIGH | LOW | P1 |
| Audiobookshelf scan trigger | HIGH | LOW | P2 |
| Chapter file download | MEDIUM | LOW | P2 |
| Dry-run mode | MEDIUM | LOW | P2 |
| JSON output mode | MEDIUM | LOW | P2 |
| Headless/daemon mode | HIGH | MEDIUM | P3 |
| Goodreads sync | LOW | HIGH | P3 |
| Configurable naming templates | LOW | MEDIUM | P3 |

**Priority key:**
- P1: Must have for launch
- P2: Should have, add when possible
- P3: Nice to have, future consideration

## Competitor Feature Analysis

| Feature | Libation | OpenAudible | audible-cli (raw) | Earworm (planned) |
|---------|----------|-------------|--------------------|--------------------|
| Authentication | Built-in (C#) | Built-in (Java) | Built-in (Python) | Wraps audible-cli |
| Library listing | GUI grid view | GUI list view | `audible library list/export` | CLI + JSON output |
| Download | GUI batch with progress | GUI batch | `audible download` per book | Orchestrated batch with queue resume |
| DRM removal | Built-in decryption | Built-in (paid feature) | Built-in AAXC handling | Delegates to audible-cli |
| Format conversion | M4B, MP3, chapter split | MP3, M4B (paid) | M4A/AAXC native | M4A only (v1) |
| Folder organization | Configurable naming templates with 20+ tags | Author/Title convention | `--output-dir` flag, manual | Libation-compatible convention |
| Fault tolerance | Poor -- crashes on large batches, DB corruption | Unknown | Per-file resume for AAXC | Queue-level resume + per-file resume |
| Headless / CLI | No (GUI only, hangup mode exists but limited) | No (GUI only) | Yes (CLI native) | Yes (CLI native, daemon planned) |
| Audiobookshelf integration | None | None | None | API scan trigger |
| Goodreads integration | None | None | None | Planned (v2) |
| Cover art | Downloaded with book | Downloaded with book | `--cover` flag | Automatic with download |
| Chapter metadata | Cue file generation, chapter split | Chapter split (paid) | `--chapter` flag | Chapter JSON download |
| New book detection | Auto-scan on launch | Manual refresh | Manual (re-export library) | Diff-based detection |
| NAS / server use | Possible but GUI-dependent, unreliable | GUI-dependent | Manual scripting required | First-class support |
| Rate limiting | Unknown / implicit | Unknown | None explicit | Configurable delays + backoff |

## Sources

- [Libation GitHub](https://github.com/rmcrackan/Libation) -- feature set, naming templates, known issues
- [Libation naming templates docs](https://getlibation.com/docs/features/naming-templates) -- folder structure conventions and metadata tags
- [audible-cli GitHub](https://github.com/mkb79/audible-cli) -- CLI commands, download resume, library export
- [OpenAudible](https://openaudible.org/) -- feature overview, pricing model
- [Audiobookshelf API docs](https://api.audiobookshelf.org/) -- library scan endpoint, authentication, metadata endpoints
- [Audiobookshelf folder structure docs](https://www.audiobookshelf.org/docs/) -- Author/Title naming convention
- [good-audible-story-sync](https://github.com/cheshire137/good-audible-story-sync) -- Goodreads/StoryGraph sync CLI tool
- Libation crash issues: [#918](https://github.com/rmcrackan/Libation/issues/918), [#886](https://github.com/rmcrackan/Libation/issues/886), [#1170](https://github.com/rmcrackan/Libation/issues/1170), [#1258](https://github.com/rmcrackan/Libation/issues/1258)

---
*Feature research for: CLI audiobook library management (Audible ecosystem)*
*Researched: 2026-04-03*
