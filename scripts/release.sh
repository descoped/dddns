#!/usr/bin/env bash
# Release a new dddns version. Run it through just:
#
#   just release 0.3.4             # checks, shows the notes, asks once, then releases
#   just release 0.3.4 --dry-run   # checks and shows the notes; changes nothing
#
# It stops, with nothing changed, unless: you are on main, in step with GitHub, with no local changes;
# the version is newer than the last tag; CI on main passed; `just check` passes; and CHANGELOG.md has
# notes under [Unreleased]. Then it moves those notes under the version, commits, tags, pushes, and
# waits for GitHub Actions (goreleaser.yml) to publish the release.
set -euo pipefail

repo=descoped/dddns
die() { echo "✗ $*" >&2; exit 1; }
step() { echo "• $*"; }

here="$(cd "$(dirname "$0")" && pwd)"
cd "$(git rev-parse --show-toplevel)"
version="${1:-}"; version="${version#v}"
dry=false; [ "${2:-}" = "--dry-run" ] && dry=true
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "usage: just release <x.y.z> [--dry-run]"
tag="v$version"

step "Checking the repository"
[ "$(git branch --show-current)" = main ] || die "switch to main first: git switch main"
[ -z "$(git status --porcelain)" ] || die "you have local changes; commit or stash them first"
git fetch -q --tags origin
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] || die "main differs from GitHub; run git pull (or push your commits)"
git rev-parse -q --verify "refs/tags/$tag" >/dev/null && die "$tag already exists"
last="$(git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null || echo v0.0.0)"
[ "$(printf '%s\n%s\n' "$last" "$tag" | sort -V | tail -1)" = "$tag" ] || die "$tag is not newer than $last"

step "Checking CI on main"
ci="$(gh run list -R "$repo" --workflow ci.yml --branch main --limit 1 --json status,conclusion,url --jq '.[0] | "\(.status) \(.conclusion) \(.url)"')"
case "$ci" in
  "completed success "*) ;;
  "") die "no CI run found for main" ;;
  completed*) die "the last CI run on main failed: ${ci##* }" ;;
  *) die "CI is still running on main; try again when it is done: ${ci##* }" ;;
esac

step "Running the tests (just check)"
log="$(mktemp)"; trap 'rm -f "$log" "$log.md"' EXIT
just check >"$log" 2>&1 || { tail -30 "$log"; die "just check failed"; }
[ -z "$(git status --porcelain)" ] || die "just check reformatted files; commit that first"

python3 -I "$here/release-changelog.py" CHANGELOG.md "$tag" "$(date +%F)" >"$log.md" 2>"$log" || { cat "$log" >&2; exit 1; }
echo
echo "Release notes for $tag (previous: $last):"
sed 's/^/    /' "$log"
echo

if $dry; then echo "Dry run: nothing changed."; exit 0; fi
[ -t 0 ] || die "run this in a terminal; it asks before releasing"
read -r -p "Release $tag? [y/N] " answer
[[ "$answer" =~ ^[yY] ]] || die "stopped; nothing changed"

step "Committing the changelog, tagging $tag and pushing"
cp "$log.md" CHANGELOG.md
git commit -q -m "docs(changelog): $tag" -- CHANGELOG.md
git tag -a "$tag" -m "dddns $tag"
git push -q --atomic origin main "$tag"

step "Waiting for GitHub to publish the release"
run=""
for _ in $(seq 1 30); do
  run="$(gh run list -R "$repo" --workflow goreleaser.yml --limit 5 --json databaseId,headBranch --jq ".[] | select(.headBranch == \"$tag\") | .databaseId" | head -1)"
  [ -n "$run" ] && break
  sleep 4
done
[ -n "$run" ] || die "the tag is pushed, but no release build started; see https://github.com/$repo/actions"
gh run watch "$run" -R "$repo" --exit-status >/dev/null 2>&1 \
  || die "the tag is pushed, but the release build failed: https://github.com/$repo/actions/runs/$run (fix it, then: gh run rerun $run -R $repo)"

echo
echo "✓ dddns $tag is released: https://github.com/$repo/releases/tag/$tag"
echo "  To put it on AWS: cd ../descoped-deploy && dddns/prod/deploy.sh $version"
