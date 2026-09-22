# Maintaining trappar/Explo

This is Jeff's personal downstream fork. Work here is intended for this fork;
do not open upstream issues/PRs or seek upstream maintainer approval. The upstream
CONTRIBUTING.md describes their community process, not this fork's workflow.

Read FORK.md before updating upstream, changing download/playlist behavior, or
building a deployment. The permanent integration/default branch is `dev` and its
push target is `origin` (trappar/Explo). `upstream` (LumePart/Explo) is fetch-only.

Merge official releases into dev through a temporary maintenance branch. Preserve
published history: do not reset dev to upstream, rebase away fork commits, force
push dev, or use GitHub's discard-changes sync option. Keep ordinary changes as
focused commits on dev or short-lived branches merged back into dev.

Preserve the behavioral guarantees listed in FORK.md and their regression tests.
Resolve merge conflicts by understanding both behaviors, not by choosing an entire
side. If upstream supersedes a fix, remove redundant implementation only after the
same behavior is covered and tests pass. Update FORK.md when a guarantee changes.

Run `scripts/check.sh` after application changes or upstream merges. CI runs the
same checks; Docker builds also run Go tests. Never weaken tests to make a merge
pass. Build NAS images from committed source with `scripts/build-nas.sh`; deployed
source snapshots are generated artifacts, not working checkouts. Wait for an active
playlist run to finish before recreating Explo. Preserve the parent Downloads mount
and SLSKD_DIR mapping described in FORK.md.

Do not commit credentials, .env files, download caches, logs, or playlist exports.
