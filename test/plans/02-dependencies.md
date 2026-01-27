# Plan: Dependencies Test

## Context
Integration test: verify Ralph respects task dependencies.
T2 depends on T1, so T1 must complete first.

## Tasks

### T1: Create first file
> Must complete before T2 can start

**Requires:** —
**Status:** open

**Done when:**
- [ ] File `output/first.txt` exists with content "step-1-done"

**Subtasks:**
1. [ ] Create `output/first.txt` containing "step-1-done"

---

### T2: Create second file
> Depends on T1 being complete

**Requires:** T1
**Status:** blocked

**Done when:**
- [ ] File `output/second.txt` exists with content "step-2-done"

**Subtasks:**
1. [ ] Create `output/second.txt` containing "step-2-done"

## Discovered
