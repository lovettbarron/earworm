# Requirements: Earworm

**Defined:** 2026-04-03
**Core Value:** Reliably download and organize Audible audiobooks into a local library with zero manual intervention

## v1 Requirements

### Library Management

- [x] **LIB-01**: User can scan an existing local audiobook directory and index discovered books by ASIN
- [x] **LIB-02**: User can view the current state of their library (books, download status, metadata)
- [x] **LIB-03**: Library state persists in a local SQLite database (not on NAS mount)
- [ ] **LIB-04**: User can configure the library root path (NAS mount or local directory)
- [ ] **LIB-05**: User can preview what would be downloaded without downloading (dry-run mode)
- [x] **LIB-06**: User can get machine-readable JSON output from all list/status commands

### Audible Integration

- [x] **AUD-01**: User can authenticate with Audible via wrapped audible-cli subprocess
- [x] **AUD-02**: User can list all books in their Audible account
- [ ] **AUD-03**: User can sync their Audible library metadata to the local database
- [ ] **AUD-04**: User can detect new books available in Audible but not yet downloaded locally

### Download Pipeline

- [x] **DL-01**: User can download audiobooks from Audible in M4A format via audible-cli
- [x] **DL-02**: Downloads include cover art saved alongside audio files
- [x] **DL-03**: Downloads include chapter metadata JSON alongside audio files
- [x] **DL-04**: Downloads are rate-limited with configurable delays between requests
- [x] **DL-05**: Downloads use exponential backoff on errors to avoid Audible throttling
- [x] **DL-06**: User sees per-book and overall progress during batch downloads
- [x] **DL-07**: Batch downloads survive process interruptions and resume from the last incomplete book
- [x] **DL-08**: Failed downloads are tracked and can be retried without re-downloading successful books
- [x] **DL-09**: Downloads go to a local staging directory first, then move to the library location

### File Organization

- [x] **ORG-01**: Downloaded books are organized in Libation-compatible folder structure (Author/Title [ASIN]/)
- [x] **ORG-02**: Cover art, chapter metadata, and audio files are placed in the correct locations within each book folder
- [x] **ORG-03**: File moves from staging to library handle cross-filesystem boundaries (copy-then-delete)

### Integrations

- [ ] **INT-01**: User can trigger an Audiobookshelf library scan via its REST API after downloads complete
- [ ] **INT-02**: User can configure Audiobookshelf connection (API URL, Bearer token, library ID)
- [ ] **INT-03**: User can sync their Audible library to Goodreads via external CLI tooling
- [x] **INT-04**: User can run Earworm in polling/daemon mode to periodically check for and download new books

### CLI & Documentation

- [ ] **CLI-01**: User interacts via clear CLI commands (auth, sync, download, status, scan)
- [ ] **CLI-02**: User can configure all settings via config file and/or CLI flags
- [x] **CLI-03**: Error messages clearly communicate what went wrong and how to recover
- [ ] **CLI-04**: README documents installation, setup (including audible-cli dependency), and all commands
- [ ] **CLI-05**: README is updated with each phase to reflect current capabilities

### Testing

- [x] **TEST-01**: Unit tests for SQLite database layer (schema creation, CRUD operations, migrations) with table-driven tests
- [ ] **TEST-02**: Unit tests for configuration loading and validation (config file parsing, flag binding, defaults)
- [x] **TEST-03**: Unit tests for local library scanner (directory walking, ASIN extraction, metadata parsing)
- [x] **TEST-04**: Integration tests for CLI commands (earworm scan, status, --json output correctness)
- [x] **TEST-05**: Unit tests for audible-cli subprocess wrapper (command construction, output parsing, error handling) using mock/fake subprocess
- [ ] **TEST-06**: Integration tests for Audible sync flow (auth validation, library metadata sync, new book detection)
- [x] **TEST-07**: Unit tests for download pipeline logic (rate limiting, backoff calculation, retry state machine, progress tracking)
- [x] **TEST-08**: Integration tests for download fault tolerance (interrupt recovery, partial download resume, failure tracking)
- [x] **TEST-09**: Unit tests for file organization logic (path construction, cross-filesystem move, naming conventions)
- [x] **TEST-10**: Integration tests for end-to-end file organization (staging to library move, folder structure validation)
- [x] **TEST-11**: Integration tests for external integrations (Audiobookshelf API mock, Goodreads sync, daemon mode lifecycle)
- [ ] **TEST-12**: All packages maintain >80% line coverage; no phase ships without passing `go test ./...`

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
| LIB-01 | Phase 2 | Complete |
| LIB-02 | Phase 2 | Complete |
| LIB-03 | Phase 1 | Complete |
| LIB-04 | Phase 1 | Pending |
| LIB-05 | Phase 3 | Pending |
| LIB-06 | Phase 2 | Complete |
| AUD-01 | Phase 3 | Complete |
| AUD-02 | Phase 3 | Complete |
| AUD-03 | Phase 3 | Pending |
| AUD-04 | Phase 3 | Pending |
| DL-01 | Phase 4 | Complete |
| DL-02 | Phase 4 | Complete |
| DL-03 | Phase 4 | Complete |
| DL-04 | Phase 4 | Complete |
| DL-05 | Phase 4 | Complete |
| DL-06 | Phase 4 | Complete |
| DL-07 | Phase 4 | Complete |
| DL-08 | Phase 4 | Complete |
| DL-09 | Phase 4 | Complete |
| ORG-01 | Phase 5 | Complete |
| ORG-02 | Phase 5 | Complete |
| ORG-03 | Phase 5 | Complete |
| INT-01 | Phase 6 | Pending |
| INT-02 | Phase 6 | Pending |
| INT-03 | Phase 6 | Pending |
| INT-04 | Phase 6 | Complete |
| CLI-01 | Phase 1 | Pending |
| CLI-02 | Phase 1 | Pending |
| CLI-03 | Phase 2 | Complete |
| CLI-04 | Phase 1 | Pending |
| CLI-05 | Phase 6 | Pending |
| TEST-01 | Phase 1 | Complete |
| TEST-02 | Phase 1 | Pending |
| TEST-03 | Phase 2 | Complete |
| TEST-04 | Phase 2 | Complete |
| TEST-05 | Phase 3 | Complete |
| TEST-06 | Phase 3 | Pending |
| TEST-07 | Phase 4 | Complete |
| TEST-08 | Phase 4 | Complete |
| TEST-09 | Phase 5 | Complete |
| TEST-10 | Phase 5 | Complete |
| TEST-11 | Phase 6 | Complete |
| TEST-12 | All Phases | Pending |

**Coverage:**
- v1 requirements: 43 total
- Mapped to phases: 43
- Unmapped: 0

---
*Requirements defined: 2026-04-03*
*Last updated: 2026-04-03 after adding TEST-xx testing requirements*
