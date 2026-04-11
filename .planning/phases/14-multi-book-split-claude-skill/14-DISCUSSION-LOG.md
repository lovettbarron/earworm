# Phase 14: Multi-Book Split & Claude Skill - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-11
**Phase:** 14-multi-book-split-claude-skill
**Areas discussed:** Split detection strategy, Split output structure, Claude Code skill scope, Skill interaction model

---

## Split Detection Strategy

### Q1: How should earworm detect book boundaries within a multi-book folder?

| Option | Description | Selected |
|--------|-------------|----------|
| Metadata-first | Compare title/author/narrator from audio file metadata. Fall back to filename pattern analysis if metadata missing. | |
| Filename pattern only | Parse filenames for book title patterns, numbering resets, or naming conventions. | |
| Hybrid with user confirmation | Use metadata + filenames to propose groups, always require user confirmation before creating split plan. | ✓ |

**User's choice:** Hybrid with user confirmation
**Notes:** Most conservative approach — uses all available signals but never acts without user approval.

### Q2: What should happen when detection confidence is low?

| Option | Description | Selected |
|--------|-------------|----------|
| Skip and flag | Don't create a split plan — flag as 'needs manual review'. User can create manual plan via CSV import. | ✓ |
| Propose best guess | Create a split plan with best-guess grouping marked as low-confidence. | |
| Interactive prompt | Interactively ask user to assign files to groups. | |

**User's choice:** Skip and flag
**Notes:** User prefers conservative approach — don't guess when uncertain.

### Q3: Confirmation step format?

| Option | Description | Selected |
|--------|-------------|----------|
| Plan preview | Generate draft split plan, show via existing `earworm plan show` dry-run view. | ✓ |
| Interactive grouping | Show files grouped by detected book, let user reassign files between groups. | |

**User's choice:** Plan preview
**Notes:** Reuses existing plan infrastructure rather than building new TUI components.

---

## Split Output Structure

### Q4: How should split book directories be named?

| Option | Description | Selected |
|--------|-------------|----------|
| Libation convention | Standard Author/Title [ASIN]/ pattern from organize module. | ✓ |
| Inherit parent naming | Base new directory name on parent folder name with suffix. | |

**User's choice:** Libation convention

### Q5: What should happen to shared files?

| Option | Description | Selected |
|--------|-------------|----------|
| Copy to all | Copy shared files into each split directory. Each book self-contained. | ✓ |
| Move to first book | Move shared files to first detected book directory. | |
| Leave in place | Leave shared files in original directory. | |

**User's choice:** Copy to all

### Q6: After splitting, should the original parent directory be auto-removed?

| Option | Description | Selected |
|--------|-------------|----------|
| Auto-remove if empty | Remove empty directory as part of split plan. | |
| Leave for cleanup command | Don't touch parent directory. User runs `earworm cleanup` separately. | ✓ |

**User's choice:** Leave for cleanup command
**Notes:** Conservative — cleanup is a separate concern.

---

## Claude Code Skill Scope

### Q7: Which commands should the skill orchestrate?

| Option | Description | Selected |
|--------|-------------|----------|
| scan (deep scan) | Run `earworm scan --deep` to detect library issues. | ✓ |
| plan create | Create plans from scan results or user descriptions. | ✓ |
| plan show (dry-run) | Preview plans for user review. Read-only. | ✓ |
| status/list | Show library state, book list, scan issues. | ✓ |

**User's choice:** All four commands

### Q8: Skill format?

| Option | Description | Selected |
|--------|-------------|----------|
| SKILL.md in .claude/skills/ | Standard Claude Code skill format. Auto-discovered. | ✓ |
| CLAUDE.md instructions only | Add skill behavior as CLAUDE.md instructions. | |

**User's choice:** SKILL.md

### Q9: Guardrail approach?

| Option | Description | Selected |
|--------|-------------|----------|
| Explicit deny-list | SKILL.md includes NEVER section for forbidden commands. | ✓ |
| Allow-list only | Only list allowed commands, rely on implicit denial. | |

**User's choice:** Explicit deny-list

---

## Skill Interaction Model

### Q10: How should users invoke the skill?

| Option | Description | Selected |
|--------|-------------|----------|
| Natural language trigger | User says 'help me clean up my library'. | |
| Slash command | User types `/earworm scan`. | |
| Both | Slash command for direct actions, natural language for exploratory. | ✓ |

**User's choice:** Both

### Q11: How should the skill present plans for review?

| Option | Description | Selected |
|--------|-------------|----------|
| Show plan + ask approval | Skill formats plan output conversationally, asks for approval. | ✓ |
| Create and notify | Create silently, tell user to review separately. | |
| Step-by-step walkthrough | Walk through each operation one by one. | |

**User's choice:** Show plan + ask approval

---

## Claude's Discretion

- Internal implementation of split detection algorithm
- SKILL.md trigger pattern specifics
- JSON output parsing and conversational formatting

## Deferred Ideas

None — discussion stayed within phase scope
