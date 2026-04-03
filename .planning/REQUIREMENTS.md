# Requirements: Earworm

**Defined:** 2026-04-03
**Core Value:** Reliably download and organize Audible audiobooks into a local library with zero manual intervention

## v1 Requirements

### Library Management

- [ ] **LIB-01**: User can scan an existing local audiobook directory and index discovered books by ASIN
- [ ] **LIB-02**: User can view the current state of their library (books, download status, metadata)
- [ ] **LIB-03**: Library state persists in a local SQLite database (not on NAS mount)
- [ ] **LIB-04**: User can configure the library root path (NAS mount or local directory)
- [ ] **LIB-05**: User can preview what would be downloaded without downloading (dry-run mode)
- [ ] **LIB-06**: User can get machine-readable JSON output from all list/status commands

### Audible Integration

- [ ] **AUD-01**: User can authenticate with Audible via wrapped audible-cli subprocess
- [ ] **AUD-02**: User can list all books in their Audible account
- [ ] **AUD-03**: User can sync their Audible library metadata to the local database
- [ ] **AUD-04**: User can detect new books available in Audible but not yet downloaded locally

### Download Pipeline

- [ ] **DL-01**: User can download audiobooks from Audible in M4A format via audible-cli
- [ ] **DL-02**: Downloads include cover art saved alongside audio files
- [ ] **DL-03**: Downloads include chapter metadata JSON alongside audio files
- [ ] **DL-04**: Downloads are rate-limited with configurable delays between requests
- [ ] **DL-05**: Downloads use exponential backoff on errors to avoid Audible throttling
- [ ] **DL-06**: User sees per-book and overall progress during batch downloads
- [ ] **DL-07**: Batch downloads survive process interruptions and resume from the last incomplete book
- [ ] **DL-08**: Failed downloads are tracked and can be retried without re-downloading successful books
- [ ] **DL-09**: Downloads go to a local staging directory first, then move to the library location

### File Organization

- [ ] **ORG-01**: Downloaded books are organized in Libation-compatible folder structure (Author/Title [ASIN]/)
- [ ] **ORG-02**: Cover art, chapter metadata, and audio files are placed in the correct locations within each book folder
- [ ] **ORG-03**: File moves from staging to library handle cross-filesystem boundaries (copy-then-delete)

### Integrations

- [ ] **INT-01**: User can trigger an Audiobookshelf library scan via its REST API after downloads complete
- [ ] **INT-02**: User can configure Audiobookshelf connection (API URL, Bearer token, library ID)
- [ ] **INT-03**: User can sync their Audible library to Goodreads via external CLI tooling
- [ ] **INT-04**: User can run Earworm in polling/daemon mode to periodically check for and download new books

### CLI & Documentation

- [ ] **CLI-01**: User interacts via clear CLI commands (auth, sync, download, status, scan)
- [ ] **CLI-02**: User can configure all settings via config file and/or CLI flags
- [ ] **CLI-03**: Error messages clearly communicate what went wrong and how to recover
- [ ] **CLI-04**: README documents installation, setup (including audible-cli dependency), and all commands
- [ ] **CLI-05**: README is updated with each phase to reflect current capabilities

## v2 Requirements

### Advanced Features

- **ADV-01**: Configurable naming templates for folder structure (beyond default Libation convention)
- **ADV-02**: Multi-region Audible marketplace support
- **ADV-03**: Post-download hook scripts for custom notifications/integrations
- **ADV-04**: Multi-format output support (M4B, MP3 via ffmpeg)

## Out of Scope

| Feature | Reason |
|---------|--------|
| AAX/AAXC decryption | Delegated to audible-cli; license contamination risk |
| Audio playback | Earworm is a library manager; Audiobookshelf handles playback |
| GUI / rich TUI | CLI-only for reliability; JSON output enables custom dashboards |
| Direct Audible API implementation | audible-cli has years of auth/DRM handling; wrapping is safer |
| Multi-service support (Libro.fm, etc.) | Scope explosion; v1 is Audible-only |
| Automatic library reorganization | Destructive on user files; only organize files Earworm downloads |
| Real-time webhook notifications | Scope creep; users can wrap CLI with their own notification scripts |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| LIB-01 | TBD | Pending |
| LIB-02 | TBD | Pending |
| LIB-03 | TBD | Pending |
| LIB-04 | TBD | Pending |
| LIB-05 | TBD | Pending |
| LIB-06 | TBD | Pending |
| AUD-01 | TBD | Pending |
| AUD-02 | TBD | Pending |
| AUD-03 | TBD | Pending |
| AUD-04 | TBD | Pending |
| DL-01 | TBD | Pending |
| DL-02 | TBD | Pending |
| DL-03 | TBD | Pending |
| DL-04 | TBD | Pending |
| DL-05 | TBD | Pending |
| DL-06 | TBD | Pending |
| DL-07 | TBD | Pending |
| DL-08 | TBD | Pending |
| DL-09 | TBD | Pending |
| ORG-01 | TBD | Pending |
| ORG-02 | TBD | Pending |
| ORG-03 | TBD | Pending |
| INT-01 | TBD | Pending |
| INT-02 | TBD | Pending |
| INT-03 | TBD | Pending |
| INT-04 | TBD | Pending |
| CLI-01 | TBD | Pending |
| CLI-02 | TBD | Pending |
| CLI-03 | TBD | Pending |
| CLI-04 | TBD | Pending |
| CLI-05 | TBD | Pending |

**Coverage:**
- v1 requirements: 31 total
- Mapped to phases: 0
- Unmapped: 31 (pending roadmap creation)

---
*Requirements defined: 2026-04-03*
*Last updated: 2026-04-03 after initial definition*
