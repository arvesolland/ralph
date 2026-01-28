# Test Plan: Core Principles Verification

This plan tests that Ralph follows all core principles:
1. One task at a time
2. Reads context files (CLAUDE.md, specs, plan, progress)
3. Picks next available task
4. Completes with verification
5. Updates plan
6. Updates progress log (EVERY iteration)
7. Commits all changes

---

### T1: Create first marker

**Requires:** (none)
**Status:** open

**Done when:**
- [ ] File `output/step1.txt` exists with content "step1-complete"

**Subtasks:**
- [ ] Create `output/` directory if needed
- [ ] Create `output/step1.txt` with content "step1-complete"

---

### T2: Create second marker (depends on T1)

**Requires:** T1
**Status:** open

**Done when:**
- [ ] File `output/step2.txt` exists with content "step2-complete"
- [ ] File `output/step1.txt` still exists (verify T1 wasn't broken)

**Subtasks:**
- [ ] Verify `output/step1.txt` exists (dependency check)
- [ ] Create `output/step2.txt` with content "step2-complete"

---

### T3: Create final marker (depends on T2)

**Requires:** T2
**Status:** open

**Done when:**
- [ ] File `output/final.txt` exists with content "all-done"
- [ ] Both `step1.txt` and `step2.txt` still exist

**Subtasks:**
- [ ] Verify both previous files exist
- [ ] Create `output/final.txt` with content "all-done"
