#!/usr/bin/env python3
"""Build the GitHub Release body for a given tag (used by the publish job).

Priority order (highest to lowest):

  1. `.release-body.md` if present and non-empty. Written by prepare_release.py
     in the same workflow run. (Because this job runs on a fresh checkout at
     the tag, the file will NOT exist when re-running against an already-
     published tag; we still check so a future single-run release can use it.)
  2. The body embedded in the annotated tag message between the
     `---RELEASE-BODY-START---` and `---RELEASE-BODY-END---` markers. This is
     the primary path for both the normal flow and for re-runs, because the
     release body is always stamped into the tag object itself.
  3. The matching `## [X.Y.Z]` entry in `CHANGELOG.md`.
  4. A minimal fallback body.
"""

from __future__ import annotations

import os
import re
import subprocess
from pathlib import Path

REPO_ROOT = Path.cwd()
PRECOMPUTED = REPO_ROOT / ".release-body.md"
OUT = REPO_ROOT / "release-body.md"
CHANGELOG = REPO_ROOT / "CHANGELOG.md"
BODY_START = "---RELEASE-BODY-START---"
BODY_END = "---RELEASE-BODY-END---"


def git(*args: str) -> str:
    r = subprocess.run(["git", *args], capture_output=True, text=True)
    return r.stdout


def extract_from_tag(tag: str) -> str:
    tag_msg = git("tag", "-l", "--format=%(contents)", tag)
    if BODY_START in tag_msg and BODY_END in tag_msg:
        start = tag_msg.index(BODY_START) + len(BODY_START)
        end = tag_msg.index(BODY_END, start)
        return tag_msg[start:end].strip() + "\n"
    return ""


def extract_changelog_entry(ver: str) -> str:
    if not CHANGELOG.exists():
        return ""
    text = CHANGELOG.read_text(encoding="utf-8")
    pat = re.compile(
        rf"^##\s*\[\s*{re.escape(ver)}\s*\].*?(?=^##\s*\[|\Z)",
        flags=re.MULTILINE | re.DOTALL,
    )
    m = pat.search(text)
    if not m:
        return ""
    entry = m.group(0).rstrip()
    lines = entry.splitlines()
    return "\n".join(lines[1:]).strip()


def main() -> int:
    ref = os.environ.get("GITHUB_REF", "")
    tag = os.environ.get("TAG", "").strip()
    if not tag and ref.startswith("refs/tags/"):
        tag = ref[len("refs/tags/"):]
    ver = tag.lstrip("v")

    body = ""
    if PRECOMPUTED.exists() and PRECOMPUTED.stat().st_size > 0:
        body = PRECOMPUTED.read_text(encoding="utf-8")
        print(f"Using precomputed release body for {tag} ({len(body)} chars).")
    elif tag:
        body = extract_from_tag(tag)
        if body:
            print(f"Using release body from annotated tag message ({len(body)} chars).")
    if not body and ver:
        entry = extract_changelog_entry(ver)
        if entry:
            body = f"## ADBPureFlow {tag}\n\n{entry}\n"
            print(f"Using CHANGELOG entry for {tag} ({len(body)} chars).")
    if not body:
        body = (
            f"## ADBPureFlow {tag}\n\n"
            f"Release {tag} of ADBPureFlow.\n\n"
            f"See [CHANGELOG.md](./CHANGELOG.md) for details.\n"
        )
        print(f"Using minimal fallback body for {tag}.")

    OUT.write_text(body if body.endswith("\n") else body + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
