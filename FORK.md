# Maintaining this fork

`trappar/Explo` is a permanent personal fork. There is no upstream contribution
workflow. `dev` is the default branch and contains both official code and our
changes. `origin/dev` is the authoritative version of this fork. Official code is
fetched from `upstream`; no second local branch or patch stack is needed to retain
our changes.

## Repository setup

```sh
git clone git@github.com:trappar/Explo.git
cd Explo
git remote add upstream https://github.com/LumePart/Explo.git
git remote set-url --push upstream DISABLED
git config remote.pushDefault origin
git config pull.ff only
```

The existing working checkout is `~/Dev/personal/Explo`. These remotes and settings
are already configured there. Branch names such as `fix/...` and `maintenance/...`
are temporary; merge completed work into dev and remove the temporary branch.

## Bringing in official updates

Prefer a reviewed official release tag. Current integrated release: **v1.2.0**
(commit `51449865c526c88f05b246b4d2447efc204854d2`). This marker is updated after
successful upstream merges; it is not the fork's own application version.

```sh
git switch dev
scripts/sync-upstream.sh v1.2.0   # replace with the chosen new official release
```

The helper fetches origin and upstream, fast-forwards local dev when possible,
then prepares an uncommitted merge on a maintenance branch. It does nothing when
the target is already included. It never pushes, deploys, or resolves conflicts.
An explicit `upstream/dev` or `upstream/main` can be used for an unreleased fix.

1. Read upstream release notes and the merge diff, especially downloader, playlist,
   configuration, Dockerfile, entrypoint, and workflow changes.
2. Resolve conflicts while preserving the guarantees below. Do not blanket-select
   ours/theirs. Use `git merge --abort` to abandon an unresolved integration.
3. Update the integrated-release marker above and any changed build instructions.
4. Run `scripts/check.sh`, then commit the merge. New fixes may be separate commits.
5. Return to dev, fast-forward it to the maintenance branch, and push `origin dev`.
   Delete the maintenance branch once incorporated. Never force-reset to upstream.

For example, after validating a maintenance branch:

```sh
git add <resolved-files>
git commit
git switch dev
git merge --ff-only maintenance/upstream-<target>
git push origin dev
```

## Behaviors the fork must preserve

| Behavior | Implementation | Regression coverage |
| --- | --- | --- |
| Failed file migration leaves a track eligible for YouTube fallback and retains its source for recovery | `src/downloader/monitor.go` | `TestImportFailureRemainsEligibleForFallback` |
| Failed, stalled, or missing Soulseek transfers try another distinct peer, then fall back when exhausted | `src/downloader/slskd.go`, `monitor.go` | retry, missing, stalled, exhaustion, and distinct-peer tests in `reliability_test.go` |
| Empty transfer queues permit retries; total monitor time does not reset per peer or get bypassed by progress | same files | empty-queue and maximum-duration tests |
| Navidrome/Subsonic playlists keep their IDs and metadata; update tracks only after acquisition and library scanning, create only when absent | `src/client/subsonic.go`, `client.go`, `src/main/main.go` | `src/client/playlist_test.go` |

The downloader repair originated in `f27ada5`. Keep the behavior even if upstream
changes its implementation. Add new fork-specific guarantees to this table.

Persistent playlist updates currently apply to Navidrome/Subsonic. Other server
adapters retain upstream behavior. The existing `--replace-playlist=true` default
uses stable names; false retains upstream dated naming. Do not turn that flag off
to request stable playlists. Existing playlist visibility and descriptions are
preserved when updating; configuration defaults are applied when first creating.

## Checks and builds

`scripts/check.sh` uses the Go version declared in go.mod, Node/npm (matching the
upstream Dockerfile), Python 3, and a C compiler for Go's race detector. It builds the real
web UI, runs all Go tests with the race detector, and compiles the application.
The fork CI runs this on pushes to dev, maintenance branches, and pull requests.
Upstream release/publishing jobs are gated to the official repository; this fork's
CI validates changes without automatically publishing or deploying them.

For the NAS, run `scripts/build-nas.sh` from a clean committed checkout. It exports
that commit with git archive, transfers it over SSH to
`/volume1/docker/explo/releases/<full-commit>/`, and builds the upstream Dockerfile
with tests. The resulting image is `navistack/explo:git-<12-character-commit>` and
includes its source revision. Source snapshots contain no Git database or secrets.
The script prints the exact Compose settings to select the result; it does not
restart services. Override the SSH alias using `NAS_HOST` (default: `nas`).

The live Compose project is `/volume1/docker/explo` on the NAS, mounted locally at
`/Volumes/docker/explo`. Set its explo image, build context, and VERSION argument
to the printed commit-specific values. Validate Compose, check
`GET http://127.0.0.1:7288/api/ui/run/status`, and when idle run:

```sh
cd /volume1/docker/explo
docker compose config --quiet
docker compose up -d --no-deps explo
```

Verify the running version, schedules, UI, and affected behavior. For playlist
changes, record the playlist ID before and after an update. A repeat run must keep
the ID and replace the song list without appending duplicates. Do not use
`--clean-downloads` for an ordinary verification run.

Keep these deployment settings intact:
- `/volume1/media/Music/Downloads:/slskd`
- `SLSKD_DIR=/slskd/complete/`
- credentials and schedules in the existing `data/.env`, not the source repository

Mounting the child `complete` directory directly previously left Explo bound to a
deleted inode. `README-reliability.md` in the Compose project records that incident
and the successful 50/50 playlist recovery. The older `fork/` source snapshot and
`Dockerfile.repair` are legacy deployment artifacts, not the maintained source.
Rollback selects the previous image/build context in Compose and recreates only
Explo; do not undo the corrected parent mount.
