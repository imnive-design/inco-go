#!/usr/bin/env zsh
set -euo pipefail

# inco release script
# Usage: ./scripts/release.sh [commit-message]
#   commit-message: optional, defaults to latest commit's message

cd "$(git rev-parse --show-toplevel)"

# ---------------------------------------------------------------------------
# Pre-flight checks
# ---------------------------------------------------------------------------

if [[ -n "$(git status --porcelain)" ]]; then
  echo "error: working tree not clean — commit or stash first" >&2
  exit 1
fi

branch=$(git branch --show-current)
if [[ "$branch" != "main" ]]; then
  echo "error: must be on main (currently on $branch)" >&2
  exit 1
fi

if ! command -v inco &>/dev/null; then
  echo "error: inco not found in PATH" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# Resolve commit message & release branch name
# ---------------------------------------------------------------------------

commit_msg="${1:-$(git --no-pager log -1 --format=%s)}"
# Derive branch name from message: lowercase, replace spaces/special chars
branch_name="release/$(echo "$commit_msg" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9]/-/g' | sed 's/--*/-/g' | sed 's/^-//;s/-$//' | head -c 40)"

latest_sha=$(git rev-parse --short HEAD)
echo "==> Releasing from main @ $latest_sha"
echo "    Branch:  $branch_name"
echo "    Message: $commit_msg"
echo ""

# ---------------------------------------------------------------------------
# Release
# ---------------------------------------------------------------------------

echo "==> Creating release branch..."
git checkout -b "$branch_name"

echo "==> Running inco release..."
inco release .

echo "==> Committing released files..."
git add -A
git commit -m "release: $commit_msg"

echo "==> Pushing to inco-go..."
git push --force inco-go "$branch_name:main"

# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------

echo "==> Cleaning release artifacts..."
inco release clean .

echo "==> Switching back to main..."
git checkout -f main
git branch -D "$branch_name"

# ---------------------------------------------------------------------------
# Install
# ---------------------------------------------------------------------------

echo "==> Installing latest binary..."
GOPRIVATE=github.com/imnive-design/inco-go \
GONOSUMCHECK=github.com/imnive-design/inco-go \
go install github.com/imnive-design/inco-go/cmd/inco@latest

echo ""
echo "==> Done! Installed: $(go version -m ~/go/bin/inco 2>/dev/null | grep 'mod' | awk '{print $3}')"
