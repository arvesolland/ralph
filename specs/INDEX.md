# Spec Index

| ID | Feature | Status | Path | Requires | Plan |
|----|---------|--------|------|----------|------|
| F1 | Go Rewrite | planned | [go-rewrite](go-rewrite/SPEC.md) | — | [pending](../plans/pending/go-rewrite.md) |
| F1.1 | ↳ Config & Prompt | planned | [go-rewrite/config](go-rewrite/config/SPEC.md) | — | — |
| F1.2 | ↳ Plan & Queue | planned | [go-rewrite/plan](go-rewrite/plan/SPEC.md) | F1.1 | — |
| F1.3 | ↳ Claude Runner | planned | [go-rewrite/runner](go-rewrite/runner/SPEC.md) | F1.1, F1.2 | — |
| F1.4 | ↳ Git & Worktree | planned | [go-rewrite/worktree](go-rewrite/worktree/SPEC.md) | F1.2 | — |
| F1.5 | ↳ Slack Integration | planned | [go-rewrite/slack](go-rewrite/slack/SPEC.md) | F1.3 | — |
| F1.6 | ↳ CLI & Release | planned | [go-rewrite/cli](go-rewrite/cli/SPEC.md) | F1.1-F1.5 | — |

## By Status

**In Progress:** —
**Blocked:** —
**Planned:** F1, F1.1, F1.2, F1.3, F1.4, F1.5, F1.6
**Complete:** —

## Quick Start

1. Create a new feature spec: `specs/feature-name/SPEC.md`
2. Add entry to this INDEX
3. Generate plan using the `ralph-spec-to-plan` skill
4. Run `ralph plans/current/feature-name.md` to implement

See `.claude/skills/ralph-spec/SKILL.md` for the full spec schema.
