# CI: bump Node 20 actions

Heads-up surfaced during the v0.3.0 release run. The workflow annotation on every job:

> Node.js 20 actions are deprecated. The following actions are running on Node.js 20 and may not work as expected: `actions/checkout@v4`, `actions/setup-go@v5`, `goreleaser/goreleaser-action@v6`. Actions will be forced to run with Node.js 24 by default starting June 16th, 2026. Node.js 20 will be removed from the runner on September 16th, 2026.

GitHub's [Node 20 deprecation changelog](https://github.blog/changelog/2025-09-19-deprecation-of-node-20-on-github-actions-runners/) is the source of truth.

## Timeline

- **2026-06-16** — runners default to Node 24; pre-Node-24 actions run forced on the new runtime and may break.
- **2026-09-16** — Node 20 runtime is removed entirely.

## What needs bumping

- `.github/workflows/release.yml`
  - `actions/checkout@v4` (lines 16, 41)
  - `actions/setup-go@v5` (line 21)
  - `goreleaser/goreleaser-action@v6` (line 26)
- `.github/workflows/ci.yml`
  - `actions/checkout@v4` (line 17)
  - `actions/setup-go@v5` (line 20)

## Approach

For each action, check the upstream repo for a newer major that runs on Node 24, bump to it, and verify CI plus a tag-driven release run still produce darwin/linux × amd64/arm64 archives and the `docs(changelog): vX.Y.Z` follow-up commit on `main`. If no Node-24 major exists yet for one of them, the interim escape hatch is `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24=true` at the workflow or job level.

## Done when

- Workflow runs no longer emit the Node 20 deprecation annotation.
- `go test -race ./...` still passes in CI.
- A `v*` tag push still produces the four release archives and the changelog-prepend commit.
