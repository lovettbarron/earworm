---
phase: 11-structural-operations-metadata
plan: 02
subsystem: fileops
tags: [metadata, sidecar, audiobookshelf, json]
dependency_graph:
  requires: [internal/metadata/metadata.go]
  provides: [internal/fileops/sidecar.go]
  affects: [organize]
tech_stack:
  added: []
  patterns: [ABS metadata.json sidecar, never-nil JSON arrays]
key_files:
  created:
    - internal/fileops/sidecar.go
    - internal/fileops/sidecar_test.go
  modified: []
decisions:
  - "publishedYear as string (not int) matching ABS JSON.stringify behavior"
  - "Empty arrays []string{} instead of nil to avoid null in JSON output"
  - "2-space indented JSON with trailing newline for ABS compatibility"
metrics:
  duration: 2min
  completed: 2026-04-07
---

# Phase 11 Plan 02: ABS Metadata Sidecar Writer Summary

ABS-compatible metadata.json sidecar writer with never-nil array fields and SHA-256 verified no-audio-modification guarantee.

## What Was Built

### ABSMetadata Types (internal/fileops/sidecar.go)
- `ABSMetadata` struct with exact ABS JSON field names: publishedYear (string), authors/narrators/series/genres ([]string), chapters ([]ABSChapter), tags ([]string)
- `ABSChapter` struct with id, start, end, title fields
- `SidecarFileName` constant: "metadata.json"

### BuildABSMetadata Converter
- Converts internal `metadata.BookMetadata` to ABS format
- Single strings (Author, Narrator, Series, Genre) converted to single-element slices
- Empty strings produce empty slices (never nil) ensuring `[]` not `null` in JSON
- Year=0 produces empty string publishedYear (not "0")

### WriteMetadataSidecar Writer
- Writes 2-space indented JSON (`json.MarshalIndent`) with trailing newline
- Overwrites existing metadata.json if present
- Wrapped errors with context prefix

## Test Coverage

10 tests total, all passing:
- `TestBuildABSMetadata_FullFields` - complete field mapping
- `TestBuildABSMetadata_EmptyFields` - all zero-value handling
- `TestBuildABSMetadata_ZeroYear` - year edge case
- `TestBuildABSMetadata_ArraysNeverNil` - JSON null prevention
- `TestWriteMetadataSidecar_WritesJSON` - round-trip marshal/unmarshal
- `TestWriteMetadataSidecar_PrettyPrinted` - 2-space indent verification
- `TestWriteMetadataSidecar_Overwrite` - idempotent writes
- `TestWriteMetadataSidecar_InvalidDir` - error handling
- `TestSidecarNoAudioModification` - SHA-256 audio file integrity
- `TestWriteMetadataSidecar_JSONFormat` - all required ABS keys present

## Commits

| Task | Commit | Description |
|------|--------|-------------|
| 1 | 9ecae7c | ABSMetadata types, BuildABSMetadata converter, WriteMetadataSidecar impl |
| 2 | 7b85631 | WriteMetadataSidecar tests and no-audio-modification check |

## Deviations from Plan

None - plan executed exactly as written.

## Known Stubs

None - all functionality is fully wired.

## Verification

- `go test ./internal/fileops/ -count=1` - 10/10 tests pass
- `go test ./... -count=1` - full suite green (14 packages)
- `go vet ./internal/fileops/` - clean

## Self-Check: PASSED
