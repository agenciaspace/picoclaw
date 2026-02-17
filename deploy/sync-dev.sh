#!/usr/bin/env bash
# ============================================================
# Sync Development Branch Script
# ============================================================
# Automatically sync with the development branch
#
# Usage:
#   bash deploy/sync-dev.sh
#   OR
#   make sync-dev
# ============================================================

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log()   { echo -e "${GREEN}✓${NC} $*"; }
warn()  { echo -e "${YELLOW}⚠${NC} $*"; }
error() { echo -e "${RED}✗${NC} $*"; exit 1; }
info()  { echo -e "${BLUE}ℹ${NC} $*"; }

# ── Get current directory ──────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PICOCLAW_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PICOCLAW_DIR"

# ── Configuration ──────────────────────────────────────
BRANCH="claude/hostinger-remote-deployment-TGVof"
REMOTE="origin"

# ── Functions ──────────────────────────────────────────
check_git() {
    if ! command -v git &>/dev/null; then
        error "git is not installed"
    fi
}

check_unstaged() {
    if ! git diff-index --quiet HEAD --; then
        warn "You have unstaged changes:"
        git status --short
        echo ""
        read -p "Continue anyway? (y/n): " continue
        [ "$continue" != "y" ] && error "Aborted"
    fi
}

fetch_latest() {
    info "Fetching latest changes from $REMOTE/$BRANCH..."
    if ! git fetch "$REMOTE" "$BRANCH" 2>/dev/null; then
        error "Failed to fetch from $REMOTE"
    fi
    log "Fetched latest changes"
}

show_diff() {
    info "Checking for differences..."
    DIFF_COUNT=$(git diff --stat origin/"$BRANCH" | tail -1 | awk '{print $1}')

    if [ -z "$DIFF_COUNT" ] || [ "$DIFF_COUNT" = "0" ]; then
        log "Already up to date!"
        return 1
    fi

    echo ""
    info "Changes to merge:"
    git diff --stat origin/"$BRANCH"
    echo ""

    read -p "View full diff? (y/n): " show_full
    if [ "$show_full" = "y" ]; then
        git diff origin/"$BRANCH" | head -100
        echo "... (showing first 100 lines)"
    fi

    return 0
}

merge_changes() {
    info "Merging changes from $REMOTE/$BRANCH..."

    if git merge origin/"$BRANCH" --no-edit 2>&1 | grep -q "Merge made"; then
        log "Changes merged successfully"
        return 0
    elif git merge origin/"$BRANCH" --no-edit 2>&1 | grep -q "Already up to date"; then
        log "Already up to date"
        return 1
    else
        warn "Merge completed (but may have conflicts)"
        git status
        return 2
    fi
}

check_conflicts() {
    if git diff --name-only --diff-filter=U | grep -q .; then
        warn "Merge conflicts detected!"
        echo ""
        echo "Conflicting files:"
        git diff --name-only --diff-filter=U
        echo ""
        error "Please resolve conflicts manually and run: git add . && git commit"
    fi
}

show_status() {
    echo ""
    info "Sync complete!"
    echo ""
    git log --oneline -5
    echo ""
}

# ── Main ───────────────────────────────────────────────
main() {
    echo ""
    info "🔄 Syncing with $BRANCH"
    echo ""

    check_git
    check_unstaged
    fetch_latest

    if show_diff; then
        read -p "Merge changes? (y/n): " merge_ok
        [ "$merge_ok" != "y" ] && error "Aborted"
        merge_changes
        check_conflicts
    fi

    show_status
    log "Done!"
}

# ── Run ────────────────────────────────────────────────
main
