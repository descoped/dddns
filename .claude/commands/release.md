---
description: Release a new dddns version - pick the version, write the release notes, tag, push and wait for GitHub to publish it. Run when the user says "release".
argument-hint: "[version, e.g. 0.3.4; default: decided from the commits]"
---

# Release dddns

The user saying "release" (or running `/release`) is the permission for every step below, including the
commit, tag and push. Stop and ask only where this file says so.

1. **Get main.** `git switch main && git pull --ff-only`. Stop if there are local changes.
2. **What changed.** `last=$(git describe --tags --abbrev=0 --match 'v[0-9]*')`, then
   `git log --no-merges --format='%h %s%n%b' $last..HEAD` and the PRs merged since then
   (`gh pr list --state merged --base main --search "merged:>=<date of $last>"`). Read the diffs where a
   subject is unclear. Nothing changed: say so and stop.
3. **Version.** `$ARGUMENTS` if given. Otherwise, from `$last`:
   - a breaking change (a `!` after the type, `BREAKING CHANGE`, a removed command, flag or config key,
     a config file that no longer loads): **ask the user** before going on;
   - any new feature (`feat`): the minor version, e.g. 0.3.3 → 0.4.0;
   - otherwise: the patch version, e.g. 0.3.3 → 0.3.4.
4. **Notes.** Write them under `## [Unreleased]` in `CHANGELOG.md`, keeping what is already there and
   adding what is missing. Group them under the headings used in the file (`### ✨ Features`,
   `### 🐛 Fixes`, `### 🔧 Changed`, `### 📚 Documentation`, `### 🧪 Tests`). Write for people who run
   dddns: what changed for them and why, in plain words, one bullet each. Leave out internal refactors
   unless they change behaviour. The repo is public: no account ids, hostnames, private repos or anything
   about where anyone deploys dddns.
5. **Release.** `just release <x.y.z> --yes`. It checks everything again, moves the notes under the
   version, commits, tags, pushes and waits for GitHub to publish the release with the binaries. If it
   stops, report why and fix only what it names (pull, wait for CI, ...); never skip a check.
6. **Report** the release URL and the version, and check the release page shows the notes and its
   assets: `gh release view v<x.y.z> --json body,assets --jq '.body[:300], (.assets | length)'`.
