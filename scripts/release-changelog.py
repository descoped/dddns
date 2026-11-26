"""Release notes from CHANGELOG.md.

    python3 scripts/release-changelog.py CHANGELOG.md v0.3.4 2026-12-01 > new.md
        Moves the [Unreleased] notes under a new version heading. Writes the new
        changelog to stdout and the notes to stderr. Fails when [Unreleased] is
        empty, so a release always says what changed.

    python3 scripts/release-changelog.py --notes CHANGELOG.md v0.3.4 > notes.md
        Prints the notes of one version (the GitHub release text). Fails when
        the version has no section.
"""
import re
import sys

REPO = "https://github.com/descoped/dddns"


def fail(msg):
    sys.stderr.write(f"release: {msg}\n")
    sys.exit(1)


if sys.argv[1] == "--notes":
    path, tag = sys.argv[2:4]
    text = open(path, encoding="utf-8").read()
    found = re.search(rf"^## \[{re.escape(tag)}\][^\n]*\n(.*?)(?=^## \[|^---$|\Z)", text, re.S | re.M)
    if not found or not found.group(1).strip():
        fail(f"CHANGELOG.md has no notes for {tag}")
    sys.stdout.write(found.group(1).strip() + "\n")
    sys.exit(0)

path, tag, date = sys.argv[1:4]
text = open(path, encoding="utf-8").read()

section = re.search(r"^## \[Unreleased\]\n(.*?)(?=^## \[)", text, re.S | re.M)
if not section:
    fail("CHANGELOG.md has no '## [Unreleased]' section followed by a version")
notes = section.group(1).strip()
if notes in ("", "(empty)"):
    fail("nothing under '## [Unreleased]' in CHANGELOG.md; write down what changed first")
if re.search(rf"^## \[{re.escape(tag)}\]", text, re.M):
    fail(f"CHANGELOG.md already has a {tag} section")

text = (
    text[: section.start()]
    + f"## [Unreleased]\n\n(empty)\n\n## [{tag}] - {date}\n\n{notes}\n\n"
    + text[section.end():]
)

link = re.compile(r"^\[Unreleased\]: \S+/compare/\S+\.\.\.HEAD$", re.M)
if len(link.findall(text)) != 1:
    fail("CHANGELOG.md needs exactly one '[Unreleased]: .../compare/<tag>...HEAD' link")
text = link.sub(
    f"[Unreleased]: {REPO}/compare/{tag}...HEAD\n[{tag}]: {REPO}/releases/tag/{tag}", text
)

sys.stdout.write(text)
sys.stderr.write(notes + "\n")
