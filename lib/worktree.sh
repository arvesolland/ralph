#!/bin/bash
# Ralph Worktree Management Library
# Provides isolated execution environments for plans using git worktrees
#
# CONCURRENCY PROTECTION (Three-Layer Lock):
# 1. File location lock: Plan in current/ = claimed (can't move same file twice)
# 2. Git worktree lock: Branch checked out = locked
#    ("fatal: '<branch>' is already checked out at '<path>'")
# 3. Directory lock: Worktree exists = execution in progress
#
# All three must be satisfied to start work. Any concurrent attempt hits at least one lock.

# Get the worktree directory path for a plan
# Usage: get_worktree_path "plan-name"
# Returns: Path like /project/.ralph/worktrees/feat-plan-name
get_worktree_path() {
    local plan_name="$1"
    local project_root="${PROJECT_ROOT:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"

    # Convert plan name to branch slug (feat/foo -> feat-foo)
    local branch_slug=$(get_feature_branch_from_name "$plan_name" | tr '/' '-')

    echo "$project_root/.ralph/worktrees/$branch_slug"
}

# Get feature branch name from plan name
# Usage: get_feature_branch_from_name "plan-name"
# Returns: feat/plan-name (with timestamp prefix stripped)
get_feature_branch_from_name() {
    local plan_name="$1"
    # Remove timestamp prefix if present (e.g., 20240127-143052-auth -> auth)
    plan_name=$(echo "$plan_name" | sed 's/^[0-9]\{8\}-[0-9]\{6\}-//')
    # Remove .md extension if present
    plan_name=$(echo "$plan_name" | sed 's/\.md$//')
    echo "feat/$plan_name"
}

# Check if a plan is currently locked (worktree exists = execution in progress)
# Usage: is_plan_locked "plan-name"
# Returns: 0 if locked (worktree exists), 1 if available
is_plan_locked() {
    local plan_name="$1"
    local worktree_path=$(get_worktree_path "$plan_name")

    if [[ -d "$worktree_path" ]]; then
        return 0  # Locked
    fi
    return 1  # Available
}

# Create or reuse a worktree for plan execution
# Usage: create_plan_worktree "plan-name" ["base-branch"]
# Returns: Path to the worktree
# Fails if: worktree creation fails or branch is checked out elsewhere
create_plan_worktree() {
    local plan_name="$1"
    local base_branch="${2:-main}"
    local project_root="${PROJECT_ROOT:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"
    local config_dir="${CONFIG_DIR:-$project_root/.ralph}"

    # Get configured base branch if not provided
    if [[ "$base_branch" == "main" ]] && [[ -f "$config_dir/config.yaml" ]]; then
        local configured_base=$(config_get "git.base_branch" "$config_dir/config.yaml" 2>/dev/null)
        base_branch="${configured_base:-main}"
    fi

    local feature_branch=$(get_feature_branch_from_name "$plan_name")
    local worktree_path=$(get_worktree_path "$plan_name")

    # If worktree already exists, reuse it (idempotent)
    if [[ -d "$worktree_path" ]]; then
        echo "$worktree_path"
        return 0
    fi

    # Ensure parent directory exists
    mkdir -p "$(dirname "$worktree_path")"

    # Create branch if it doesn't exist (from base branch)
    if ! git show-ref --verify --quiet "refs/heads/$feature_branch" 2>/dev/null; then
        log_info "Creating branch $feature_branch from $base_branch" >&2
        git branch "$feature_branch" "$base_branch" >&2 2>&1
    fi

    # Create worktree
    # This will fail if branch is already checked out elsewhere (git's built-in lock)
    log_info "Creating worktree at $worktree_path" >&2
    local wt_output
    if ! wt_output=$(git worktree add "$worktree_path" "$feature_branch" 2>&1); then
        log_error "Failed to create worktree for $feature_branch" >&2
        log_error "Output: $wt_output" >&2
        return 1
    fi

    echo "$worktree_path"
    return 0
}

# Remove a worktree after plan completion
# Usage: remove_plan_worktree "plan-name"
remove_plan_worktree() {
    local plan_name="$1"
    local worktree_path=$(get_worktree_path "$plan_name")

    if [[ -d "$worktree_path" ]]; then
        log_info "Removing worktree at $worktree_path" >&2
        git worktree remove "$worktree_path" --force 2>&1 >&2 || true
    fi

    # Prune stale worktree references
    git worktree prune 2>&1 >&2 || true
}

# List all plan worktrees
# Usage: list_plan_worktrees
# Output: One worktree path per line
list_plan_worktrees() {
    local project_root="${PROJECT_ROOT:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"
    local worktrees_dir="$project_root/.ralph/worktrees"

    if [[ -d "$worktrees_dir" ]]; then
        for dir in "$worktrees_dir"/*/; do
            [[ -d "$dir" ]] && echo "${dir%/}"
        done
    fi
}

# Clean up orphaned worktrees (no matching plan in current/)
# Usage: cleanup_orphan_worktrees
# Returns: Count of cleaned worktrees
cleanup_orphan_worktrees() {
    local project_root="${PROJECT_ROOT:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"
    local current_dir="$project_root/plans/current"
    local cleaned=0

    # First, let git prune any stale references
    git worktree prune 2>&1 >&2 || true

    # Then check our worktrees directory
    for worktree_path in $(list_plan_worktrees); do
        local worktree_name=$(basename "$worktree_path")
        # Convert back to plan name (feat-foo -> foo)
        local plan_name=$(echo "$worktree_name" | sed 's/^feat-//')

        # Check if a matching plan exists in current/
        local has_plan=false
        shopt -s nullglob
        for plan_file in "$current_dir"/*"$plan_name"*.md; do
            if [[ -f "$plan_file" ]] && [[ ! "$plan_file" == *.progress.md ]]; then
                has_plan=true
                break
            fi
        done
        shopt -u nullglob

        if [[ "$has_plan" == "false" ]]; then
            log_warn "Removing orphaned worktree: $worktree_path" >&2
            git worktree remove "$worktree_path" --force 2>&1 >&2 || true
            cleaned=$((cleaned + 1))
        fi
    done

    echo "$cleaned"
}

# Get the branch currently checked out in a worktree
# Usage: get_worktree_branch "/path/to/worktree"
get_worktree_branch() {
    local worktree_path="$1"

    if [[ -d "$worktree_path" ]]; then
        git -C "$worktree_path" branch --show-current 2>/dev/null
    fi
}

# Ensure worktree has latest from base branch (for long-running plans)
# Usage: update_worktree_from_base "plan-name" ["base-branch"]
update_worktree_from_base() {
    local plan_name="$1"
    local base_branch="${2:-main}"
    local worktree_path=$(get_worktree_path "$plan_name")

    if [[ ! -d "$worktree_path" ]]; then
        log_error "Worktree does not exist: $worktree_path" >&2
        return 1
    fi

    log_info "Updating worktree from $base_branch" >&2

    # Fetch latest
    git -C "$worktree_path" fetch origin "$base_branch" 2>&1 >&2 || true

    # Merge base branch (will fail if conflicts, which is expected)
    if ! git -C "$worktree_path" merge "origin/$base_branch" --no-edit 2>&1 >&2; then
        log_warn "Merge conflict when updating from $base_branch" >&2
        git -C "$worktree_path" merge --abort 2>&1 >&2 || true
        return 1
    fi

    return 0
}
