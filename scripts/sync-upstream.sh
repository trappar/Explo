#!/bin/sh
set -eu
if [ "$#" -ne 1 ]; then
  echo "Usage: scripts/sync-upstream.sh <official-tag|upstream/dev|upstream/main>" >&2
  exit 2
fi
target=$1
case "$target" in
  v[0-9]*|upstream/dev|upstream/main) ;;
  *) echo "Choose an official release tag or upstream/dev or upstream/main." >&2; exit 2 ;;
esac
cd "$(git rev-parse --show-toplevel)"
if [ "$(git branch --show-current)" != dev ]; then
  echo "Start on the fork's dev branch." >&2; exit 1
fi
if [ -n "$(git status --porcelain)" ]; then
  echo "Commit or stash existing work before an upstream merge." >&2; exit 1
fi
git fetch origin
git fetch upstream --tags
git merge --ff-only origin/dev
commit=$(git rev-parse --verify "$target^{commit}")
if git merge-base --is-ancestor "$commit" HEAD; then
  echo "$target is already included in dev."
  exit 0
fi
label=$(printf '%s' "$target" | tr / -)
branch="maintenance/upstream-$label"
git switch -c "$branch"
if ! git merge --no-ff --no-commit "$commit"; then
  echo "Resolve the merge using FORK.md, or run git merge --abort." >&2
  exit 1
fi
echo "Merge prepared on $branch. Review FORK.md, run scripts/check.sh, then commit."
