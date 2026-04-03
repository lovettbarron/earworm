---
phase: quick
plan: 260403-jqe
type: execute
wave: 1
depends_on: []
files_modified:
  - .planning/REQUIREMENTS.md
  - .planning/ROADMAP.md
autonomous: true
requirements: []
must_haves:
  truths:
    - "REQUIREMENTS.md contains explicit testing requirements (TEST-xx) covering unit and integration tests for every feature area"
    - "Every ROADMAP.md phase references testing requirement IDs and has testing success criteria"
    - "Traceability table in REQUIREMENTS.md maps each TEST-xx to its phase"
  artifacts:
    - path: ".planning/REQUIREMENTS.md"
      provides: "Testing requirements section with TEST-xx IDs"
      contains: "### Testing"
    - path: ".planning/ROADMAP.md"
      provides: "Updated phases with testing success criteria"
      contains: "TEST-"
  key_links:
    - from: ".planning/ROADMAP.md"
      to: ".planning/REQUIREMENTS.md"
      via: "TEST-xx requirement IDs referenced in phase Requirements lines"
      pattern: "TEST-\\d+"
---

<objective>
Add comprehensive testing requirements to REQUIREMENTS.md and update every ROADMAP.md phase to explicitly include unit and integration testing for features built in that phase.

Purpose: Ensure testing is a first-class deliverable in every phase, not an afterthought.
Output: Updated REQUIREMENTS.md with TEST-xx requirements, updated ROADMAP.md with testing success criteria per phase.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/REQUIREMENTS.md
@.planning/STATE.md
</context>

<tasks>

<task type="auto">
  <name>Task 1: Add testing requirements to REQUIREMENTS.md</name>
  <files>.planning/REQUIREMENTS.md</files>
  <action>
Add a new "### Testing" section to REQUIREMENTS.md under "## v1 Requirements", placed after the "### CLI & Documentation" section. Create TEST-xx requirements that cover unit and integration testing for each feature area. Requirements should follow this pattern:

- **TEST-01**: Unit tests for SQLite database layer (schema creation, CRUD operations, migrations) with table-driven tests
- **TEST-02**: Unit tests for configuration loading and validation (config file parsing, flag binding, defaults)
- **TEST-03**: Unit tests for local library scanner (directory walking, ASIN extraction, metadata parsing)
- **TEST-04**: Integration tests for CLI commands (earworm scan, status, --json output correctness)
- **TEST-05**: Unit tests for audible-cli subprocess wrapper (command construction, output parsing, error handling) using mock/fake subprocess
- **TEST-06**: Integration tests for Audible sync flow (auth validation, library metadata sync, new book detection)
- **TEST-07**: Unit tests for download pipeline logic (rate limiting, backoff calculation, retry state machine, progress tracking)
- **TEST-08**: Integration tests for download fault tolerance (interrupt recovery, partial download resume, failure tracking)
- **TEST-09**: Unit tests for file organization logic (path construction, cross-filesystem move, naming conventions)
- **TEST-10**: Integration tests for end-to-end file organization (staging to library move, folder structure validation)
- **TEST-11**: Integration tests for external integrations (Audiobookshelf API mock, Goodreads sync, daemon mode lifecycle)
- **TEST-12**: All packages maintain >80% line coverage; no phase ships without passing `go test ./...`

Also update the Traceability table at the bottom to map each TEST-xx to its phase:
- TEST-01, TEST-02 -> Phase 1
- TEST-03, TEST-04 -> Phase 2
- TEST-05, TEST-06 -> Phase 3
- TEST-07, TEST-08 -> Phase 4
- TEST-09, TEST-10 -> Phase 5
- TEST-11 -> Phase 6
- TEST-12 -> All Phases

Update the Coverage counts:
- v1 requirements total: 43 (was 31, adding 12 TEST-xx)
- Mapped to phases: 43
- Unmapped: 0
  </action>
  <verify>
    <automated>grep -c "TEST-" .planning/REQUIREMENTS.md | grep -q "^[1-9]" && echo "PASS: TEST requirements found" || echo "FAIL"</automated>
  </verify>
  <done>REQUIREMENTS.md has 12 TEST-xx requirements in a Testing section, all mapped in the traceability table, coverage counts updated</done>
</task>

<task type="auto">
  <name>Task 2: Update every ROADMAP.md phase with testing criteria and requirement IDs</name>
  <files>.planning/ROADMAP.md</files>
  <action>
Update each phase in ROADMAP.md to include testing requirements and success criteria:

**Phase 1: Foundation & Configuration**
- Add TEST-01, TEST-02 to the Requirements line (becomes: LIB-03, LIB-04, CLI-01, CLI-02, CLI-04, TEST-01, TEST-02)
- Add success criterion: "Unit tests pass for database layer (schema, CRUD) and config system (parsing, defaults, validation) via `go test ./...`"

**Phase 2: Local Library Scanning**
- Add TEST-03, TEST-04 to Requirements (becomes: LIB-01, LIB-02, LIB-06, CLI-03, TEST-03, TEST-04)
- Add success criterion: "Unit tests cover scanner logic (directory walking, ASIN extraction) and integration tests verify CLI commands (scan, status, --json) produce correct output"

**Phase 3: Audible Integration**
- Add TEST-05, TEST-06 to Requirements (becomes: AUD-01, AUD-02, AUD-03, AUD-04, LIB-05, TEST-05, TEST-06)
- Add success criterion: "Unit tests cover audible-cli wrapper (command building, output parsing, error mapping) using fake subprocess; integration tests verify sync and new-book detection flows"

**Phase 4: Download Pipeline**
- Add TEST-07, TEST-08 to Requirements (becomes: DL-01 through DL-09, TEST-07, TEST-08)
- Add success criterion: "Unit tests cover rate limiter, backoff calculator, retry state machine, and progress tracker; integration tests verify interrupt recovery and failure tracking end-to-end"

**Phase 5: File Organization**
- Add TEST-09, TEST-10 to Requirements (becomes: ORG-01, ORG-02, ORG-03, TEST-09, TEST-10)
- Add success criterion: "Unit tests cover path construction and naming logic; integration tests verify staging-to-library moves including cross-filesystem boundary handling"

**Phase 6: Integrations & Polish**
- Add TEST-11 to Requirements (becomes: INT-01, INT-02, INT-03, INT-04, CLI-05, TEST-11)
- Add success criterion: "Integration tests cover Audiobookshelf API calls (using HTTP mock), Goodreads sync trigger, and daemon mode start/stop lifecycle"

For each phase, add the new success criterion as the LAST numbered item in the existing Success Criteria list. Do not remove or reorder existing criteria.
  </action>
  <verify>
    <automated>grep -c "TEST-" .planning/ROADMAP.md | grep -q "^[1-9]" && grep "go test" .planning/ROADMAP.md > /dev/null && echo "PASS: ROADMAP has testing refs and criteria" || echo "FAIL"</automated>
  </verify>
  <done>All 6 phases in ROADMAP.md reference TEST-xx requirements and include a testing-specific success criterion</done>
</task>

</tasks>

<verification>
1. Every TEST-xx ID in REQUIREMENTS.md appears in at least one ROADMAP.md phase Requirements line
2. Every ROADMAP.md phase has at least one TEST-xx in its Requirements
3. Every ROADMAP.md phase has a testing success criterion mentioning tests
4. Traceability table covers all 43 requirements with no unmapped entries
</verification>

<success_criteria>
- REQUIREMENTS.md contains 12 new TEST-xx requirements in a Testing section
- All 6 ROADMAP.md phases reference their corresponding TEST-xx IDs
- All 6 ROADMAP.md phases have a testing-focused success criterion
- Traceability table is complete and accurate
- `go test` or testing language appears in every phase's success criteria
</success_criteria>

<output>
After completion, create `.planning/quick/260403-jqe-ensure-each-roadmap-phase-includes-compr/260403-jqe-SUMMARY.md`
</output>
