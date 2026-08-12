#!/usr/bin/env python3
"""Prepare a new release when a PR is merged into `main`.

This script is invoked from the `version-bump` job of `.github/workflows/release.yml`
and is responsible for:

  1. Detecting whether the current push is an automated release commit (in
     which case we should NOT produce a new version — this closes the loop
     that would otherwise be caused by pushing the CHANGELOG update).
  2. Finding the merge commit on `main` corresponding to the just-merged PR,
     and fetching that PR's title, body, labels, and number via the GitHub
     REST API.
  3. Determining the next semantic version based on PR labels / body content
     (major = breaking change, minor = feature, patch = fix/maintenance), with
     a sensible patch-level fallback.
  4. Parsing the PR body to extract the `## Summary`, `## Validation`,
     `## Breaking Changes`, and `## Notes` sections (case-insensitive heading
     matching, tolerant of alternate punctuation).
  5. Prepending a new entry to `CHANGELOG.md` without overwriting history.
  6. Committing the updated CHANGELOG with a message containing the magic
     marker `[release skip]` (detected in step 1), creating an annotated
     Git tag `vX.Y.Z`, and writing outputs (`tag`, `version`, `body_file`,
     `changelog_updated`) for downstream jobs.

The script is intentionally dependency-free (uses only the Python standard
library) to keep the release pipeline deterministic and free of third-party
automation.
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
import urllib.error
import urllib.request
from datetime import date, datetime, timezone
from pathlib import Path


REPO = os.environ.get("GITHUB_REPOSITORY", "flessan/AdbPureFlow")
GITHUB_TOKEN = os.environ.get("GITHUB_TOKEN", "")
HEAD_SHA = os.environ.get("HEAD_SHA", "HEAD")
API = "https://api.github.com"
RELEASE_SKIP_MARKER = "[release skip]"
CHANGELOG_PATH = Path("CHANGELOG.md")
BODY_OUT_PATH = Path(".release-body.md")

# ---------------------------------------------------------------------------
# GitHub helpers
# ---------------------------------------------------------------------------

def gh_request(method: str, path: str, data: dict | None = None) -> dict | list | None:
    url = f"{API}{path}"
    body = json.dumps(data).encode() if data is not None else None
    req = urllib.request.Request(url, data=body, method=method)
    req.add_header("Authorization", f"Bearer {GITHUB_TOKEN}")
    req.add_header("Accept", "application/vnd.github+json")
    req.add_header("X-GitHub-Api-Version", "2022-11-28")
    if body is not None:
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req) as resp:
            raw = resp.read()
            return json.loads(raw) if raw else None
    except urllib.error.HTTPError as exc:
        print(f"::error::{method} {path} -> HTTP {exc.code}: {exc.read().decode(errors='replace')}",
              file=sys.stderr)
        raise


# ---------------------------------------------------------------------------
# Git helpers
# ---------------------------------------------------------------------------

def git(*args: str, check: bool = True) -> str:
    result = subprocess.run(["git", *args], capture_output=True, text=True)
    if check and result.returncode != 0:
        raise RuntimeError(f"git {' '.join(args)} failed: {result.stderr.strip()}")
    return result.stdout.strip()


def latest_version_tag() -> tuple[str, tuple[int, int, int]]:
    """Return (tag_str, (major, minor, patch)) of the highest v* tag reachable from HEAD.

    If no tag exists, scan CHANGELOG.md for the last [X.Y.Z] heading to
    preserve continuity with existing history (the repository originally
    recorded versions manually in CHANGELOG rather than via tags).
    """
    tags = git("tag", "--list", "v[0-9]*.[0-9]*.[0-9]*", "--sort=-v:refname")
    tag_list = [t for t in tags.splitlines() if t]
    for tag in tag_list:
        m = re.match(r"^v(\d+)\.(\d+)\.(\d+)$", tag)
        if m:
            return tag, (int(m.group(1)), int(m.group(2)), int(m.group(3)))

    # Fallback: scan CHANGELOG for the last [X.Y.Z] entry.
    if CHANGELOG_PATH.exists():
        text = CHANGELOG_PATH.read_text(encoding="utf-8")
        for m in re.finditer(r"^##\s*\[(\d+)\.(\d+)\.(\d+)\]", text, flags=re.MULTILINE):
            major, minor, patch = (int(x) for x in m.groups())
            # Synthesize a virtual tag reference — we do NOT create the tag.
            return f"v{major}.{minor}.{patch}", (major, minor, patch)

    # Ultimate fallback: start at 0.1.0 so the very first automated release is
    # v0.1.0 (which is appropriate for a project that hasn't yet tagged).
    return "", (0, 0, 0)


def existing_release_tag(tag: str) -> bool:
    """Return True if a Git tag already exists for `tag` (idempotency check)."""
    result = subprocess.run(["git", "rev-parse", "--verify", tag],
                            capture_output=True, text=True)
    return result.returncode == 0


def find_merged_pr_number() -> int | None:
    """Walk back from HEAD_SHA until we find a merge commit for a PR into main.

    A GitHub PR merge commit message has one of these forms:
        Merge pull request #NNN from <branch>
        <PR title> (#NNN)                 (squash merge)
    """
    # Look at up to the last 20 commits — merges into main should be at HEAD.
    log = git("log", "--pretty=%H%n%s%n%b%n---END---", "-n", "20", HEAD_SHA)
    blocks = log.split("---END---")
    for block in blocks:
        block = block.strip()
        if not block:
            continue
        lines = block.splitlines()
        # first line after the sha is the subject; but our format is sha on its
        # own line, then subject line, then body.
        sha_line = lines[0]
        subject = lines[1] if len(lines) > 1 else ""
        body = "\n".join(lines[2:]) if len(lines) > 2 else ""

        # Skip automated release commits.
        if RELEASE_SKIP_MARKER in subject or RELEASE_SKIP_MARKER in body:
            continue

        # Merge commit form: "Merge pull request #NNN ..."
        m = re.search(r"Merge pull request #(\d+)", subject)
        if m:
            print(f"::debug::Found merge-commit PR #{m.group(1)} at {sha_line}")
            return int(m.group(1))

        # Squash-merge form: "<title> (#NNN)" at the end of the subject.
        m = re.search(r"\(#(\d+)\)\s*$", subject)
        if m:
            print(f"::debug::Found squash-merge PR #{m.group(1)} at {sha_line}")
            return int(m.group(1))

    return None


# ---------------------------------------------------------------------------
# PR body parsing
# ---------------------------------------------------------------------------

SECTION_ALIASES = {
    "summary":          "Summary",
    "description":      "Summary",        # legacy PR template heading
    "validation":       "Validation",
    "testing":          "Validation",
    "tests":            "Validation",
    "breaking changes": "Breaking Changes",
    "breaking change":  "Breaking Changes",
    "notes":            "Notes",
    "additional notes": "Notes",
    "separator":        "_Separator",     # template `<!-- SEPARATOR -->` placeholder
    "type of change":   "_Type",          # checklist — drop from release notes
    "checklist":        "_Checklist",
    "screenshots":      "_Screenshots",
    "screenshots / interactive gifs": "_Screenshots",
}

HEADING_RE = re.compile(r"^(#{1,6})\s*(.+?)\s*$")
# HTML comments (including multi-line), stripped so PR template guidance is
# not emitted into release notes/changelogs.
HTML_COMMENT_RE = re.compile(r"<!--.*?-->", flags=re.DOTALL)


def parse_sections(body: str) -> dict[str, str]:
    """Parse a Markdown body into a dict keyed by canonical section name.

    Sections start with an ATX heading (`##`, `###`, ...). We treat all heading
    levels equally and look them up case-insensitively via SECTION_ALIASES.
    Content before the first heading is collected under "Summary" as a
    fallback. HTML comments (`<!-- ... -->`) are stripped so explanatory text
    from the PR template doesn't leak into release notes.
    """
    # Strip HTML comments first (works across line breaks).
    body = HTML_COMMENT_RE.sub("", body)

    sections: dict[str, str] = {}
    current_key = "Summary"
    current_buf: list[str] = []

    def flush():
        content = "\n".join(current_buf).strip()
        # Collapse runs of >2 blank lines and trim filler lines that the PR
        # template leaves behind (e.g. bare "None." placeholders are kept;
        # pure whitespace is not).
        if content:
            # If the key is prefixed with '_' it's a throwaway section.
            if not current_key.startswith("_"):
                sections.setdefault(current_key, content)
        current_buf.clear()

    for line in body.splitlines():
        m = HEADING_RE.match(line)
        if m:
            flush()
            raw_heading = m.group(2).strip().lower()
            key = SECTION_ALIASES.get(raw_heading, raw_heading.title() if raw_heading else "")
            current_key = key
            continue
        current_buf.append(line)

    flush()
    return sections


# ---------------------------------------------------------------------------
# Semver bump logic
# ---------------------------------------------------------------------------

LABEL_BUMP_MAJOR = {"breaking", "breaking-change", "breaking change", "major"}
LABEL_BUMP_MINOR = {"feature", "enhancement", "minor", "new-feature", "feature request"}
LABEL_BUMP_PATCH = {"bug", "bugfix", "fix", "patch", "maintenance", "chore", "ci", "docs", "documentation"}


def determine_bump(pr: dict) -> str:
    """Return 'major', 'minor', or 'patch' based on the PR's labels and body.

    Labels take precedence over body keywords. If nothing indicates a bump
    level, default to 'patch'.
    """
    labels = {lbl["name"].lower() for lbl in pr.get("labels", [])}

    body_text = (pr.get("body") or "")
    body_lower = body_text.lower()

    # Explicit release-type markers in body, e.g. `Release-As: v5.1.0` or
    # `Semver: major`. These take precedence over everything except a
    # "breaking" label.
    m = re.search(r"^release-as:\s*v?\d+\.\d+\.\d+\s*$", body_lower, flags=re.MULTILINE)
    if m:
        return "explicit"
    m = re.search(r"^semver:\s*(major|minor|patch)\s*$", body_lower, flags=re.MULTILINE)
    if m:
        return m.group(1)

    # Parse the body once into sections so we can scope checkboxes to the
    # `Type of Change` area rather than matching "[x] ... breaking" in the
    # Validation checklist.
    sections = parse_sections(body_text)
    type_section_parts = [sections.get(k, "") for k in (
        "_Type", "Type Of Change", "Type of Change", "Type",
    )]
    # Also scan the raw body between a "## Type of Change" heading and the
    # next heading (in case an alias didn't match).
    in_toc = False
    toc_lines: list[str] = []
    for line in body_text.splitlines():
        hm = HEADING_RE.match(line)
        if hm:
            if in_toc:
                break
            if hm.group(2).strip().lower() in {"type of change", "type"}:
                in_toc = True
            continue
        if in_toc:
            toc_lines.append(line)
    type_section_text = "\n".join(type_section_parts + toc_lines).lower()

    checked = {option for option in ("major", "minor", "patch")
               if re.search(rf"-\s*\[\s*x\s*\].{{0,80}}{option}", type_section_text,
                            flags=re.IGNORECASE)}

    # Conventional-Commits-style `BREAKING CHANGE:` footer anywhere in body.
    if "breaking change:" in body_lower:
        return "major"

    if labels & LABEL_BUMP_MAJOR or "major" in checked:
        return "major"
    if labels & LABEL_BUMP_MINOR or "minor" in checked:
        return "minor"
    if labels & LABEL_BUMP_PATCH or "patch" in checked:
        return "patch"
    # Default: safe, conservative patch bump.
    return "patch"


def next_version(current: tuple[int, int, int], bump: str) -> tuple[int, int, int]:
    major, minor, patch = current
    if bump == "major":
        return (major + 1, 0, 0)
    if bump == "minor":
        return (major, minor + 1, 0)
    # patch / explicit-as-patch / fallback
    return (major, minor, patch + 1)


def explicit_version(body: str) -> tuple[int, int, int] | None:
    m = re.search(r"^release-as:\s*v?(\d+)\.(\d+)\.(\d+)\s*$",
                  (body or ""), flags=re.MULTILINE | re.IGNORECASE)
    if m:
        return (int(m.group(1)), int(m.group(2)), int(m.group(3)))
    return None


# ---------------------------------------------------------------------------
# Changelog
# ---------------------------------------------------------------------------

_PLACEHOLDER_LINES = {
    "none.", "none", "n/a", "na", "-",
    "- replace this bullet with a summary of your change.",
}


def _has_user_content(text: str | None) -> bool:
    """Return True if a section contains meaningful user-provided content,
    as opposed to being empty or containing only the PR template boilerplate.
    """
    if not text:
        return False
    stripped = text.strip()
    if not stripped:
        return False
    # Treat pure placeholder ("None.") as empty for release-note purposes.
    if stripped.lower() in _PLACEHOLDER_LINES:
        return False
    # If every non-empty line is a template reminder bullet, treat as empty.
    lines = [ln.strip() for ln in stripped.splitlines() if ln.strip()]
    if not lines:
        return False
    return any(ln.lower() not in _PLACEHOLDER_LINES and not ln.startswith("<!--")
               for ln in lines)


def build_changelog_entry(version: str, pr: dict, sections: dict[str, str]) -> str:
    today = date.today().isoformat()
    pr_num = pr["number"]
    title = (pr.get("title") or "").strip()
    author = (pr.get("user") or {}).get("login", "unknown")

    lines = [f"## [{version[1:]}] - {today}", ""]
    if title:
        lines.append(f"### {title}")
        lines.append("")

    if "Summary" in sections:
        lines.append(sections["Summary"].rstrip())
        lines.append("")

    if _has_user_content(sections.get("Breaking Changes")):
        lines.append("### ⚠️ Breaking Changes")
        lines.append("")
        lines.append(sections["Breaking Changes"].rstrip())
        lines.append("")

    if _has_user_content(sections.get("Validation")):
        lines.append("### Validation")
        lines.append("")
        lines.append(sections["Validation"].rstrip())
        lines.append("")

    if _has_user_content(sections.get("Notes")):
        lines.append("### Notes")
        lines.append("")
        lines.append(sections["Notes"].rstrip())
        lines.append("")

    lines.append(f"**Pull Request:** [#{pr_num}]({pr.get('html_url', '')}) "
                 f"by @{author}")
    lines.append("")
    return "\n".join(lines)


def prepend_changelog(entry: str) -> None:
    entry = entry.rstrip() + "\n\n---\n\n"
    if CHANGELOG_PATH.exists():
        existing = CHANGELOG_PATH.read_text(encoding="utf-8")
        # Find the first `## [` heading after the header and insert before it,
        # preserving the introductory "All notable changes..." paragraph and
        # any `---` separator that precedes the entries.
        m = re.search(r"^##\s*\[\d+\.\d+\.\d+\]", existing, flags=re.MULTILINE)
        if m:
            # Back up over a preceding separator line + blank lines so the new
            # entry's own separator cleanly replaces it (no duplicated "---").
            insert_at = m.start()
            prefix = existing[:insert_at].rstrip()
            # If the prefix ends with "---", keep it that way and just append;
            # otherwise ensure a blank line between header and new entry.
            if prefix.endswith("---"):
                new_text = prefix + "\n\n" + entry + existing[insert_at:].lstrip()
            else:
                new_text = existing[:insert_at] + entry + existing[insert_at:]
        else:
            new_text = existing.rstrip() + "\n\n" + entry
    else:
        new_text = (
            "# Changelog\n\n"
            "All notable changes to this project will be documented in this file.\n\n"
            "The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),\n"
            "and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).\n\n"
            "---\n\n"
            + entry
        )
    CHANGELOG_PATH.write_text(new_text, encoding="utf-8")


# ---------------------------------------------------------------------------
# Release body (GitHub release notes)
# ---------------------------------------------------------------------------

def _embed_body_in_tag(tag: str, pr: dict, sections: dict[str, str], version: str,
                      create: bool = False) -> None:
    """Embed the rendered release body in the annotated tag message between
    two markers, so the publish job can recover it even on a fresh checkout.

    If `create` is True the tag is freshly created with `git tag -a`; otherwise
    an existing tag is replaced (via `git tag -a -f`) so that a re-run can
    update the body.
    """
    release_body = build_release_body(pr, sections, version)
    pr_number = pr["number"]
    tag_msg_parts = [
        f"Release generated from PR #{pr_number}: {pr.get('title','').strip()}",
        "",
        "---RELEASE-BODY-START---",
        release_body.rstrip(),
        "---RELEASE-BODY-END---",
    ]
    tag_msg = "\n".join(tag_msg_parts) + "\n"
    tag_msg_file = Path(".git") / "TAG_MESSAGE"
    tag_msg_file.write_text(tag_msg, encoding="utf-8")
    try:
        args = ["git", "tag"]
        args += ["-a"]
        if not create:
            args += ["-f"]
        args += [tag, "-F", str(tag_msg_file), "--cleanup=whitespace"]
        subprocess.run(args, check=True)
    finally:
        try:
            tag_msg_file.unlink()
        except OSError:
            pass


def build_release_body(pr: dict, sections: dict[str, str], version: str) -> str:
    pr_num = pr["number"]
    title = (pr.get("title") or "").strip()
    author = (pr.get("user") or {}).get("login", "unknown")
    url = pr.get("html_url", "")

    out: list[str] = []
    out.append(f"## ADBPureFlow {version}")
    out.append("")
    if title:
        out.append(f"**{title}**")
        out.append("")
    if _has_user_content(sections.get("Summary")):
        out.append(sections["Summary"].rstrip())
        out.append("")
    if _has_user_content(sections.get("Breaking Changes")):
        out.append("### ⚠️ Breaking Changes")
        out.append("")
        out.append(sections["Breaking Changes"].rstrip())
        out.append("")
    if _has_user_content(sections.get("Validation")):
        out.append("### Validation")
        out.append("")
        out.append(sections["Validation"].rstrip())
        out.append("")
    if _has_user_content(sections.get("Notes")):
        out.append("### Additional Notes")
        out.append("")
        out.append(sections["Notes"].rstrip())
        out.append("")
    out.append("---")
    out.append("")
    out.append(f"- Merged via Pull Request: [#{pr_num}]({url})")
    out.append(f"- Author: @{author}")
    out.append(f"- Released on: {datetime.now(timezone.utc).strftime('%Y-%m-%d %H:%M UTC')}")
    out.append("")
    return "\n".join(out)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def set_output(key: str, value: str) -> None:
    """Write a step output via $GITHUB_OUTPUT using the heredoc delimiter
    syntax so multi-line values (e.g. the body_file path is just a single line
    here, but we use the safe form anyway) are handled correctly.
    """
    value = value or ""
    out_file = os.environ.get("GITHUB_OUTPUT")
    if out_file:
        delim = f"gho_{key}_{os.urandom(6).hex()}"
        with open(out_file, "a", encoding="utf-8") as f:
            f.write(f"{key}<<{delim}\n{value}\n{delim}\n")
    # Log for visibility in the Action run log.
    print(f"::notice::output {key}={value!r}")


def main() -> int:
    # --- Guard: skip automated release commits (close the loop) --------------
    head_subject = git("log", "-1", "--pretty=%s", HEAD_SHA)
    head_body = git("log", "-1", "--pretty=%b", HEAD_SHA)
    if RELEASE_SKIP_MARKER in head_subject or RELEASE_SKIP_MARKER in head_body:
        print("::notice::HEAD is an automated release commit; nothing to do.")
        set_output("changelog_updated", "false")
        set_output("tag", "")
        set_output("version", "")
        set_output("skip_build", "true")
        return 0

    # --- Optional: bail out early if the caller already supplied a tag -------
    # (set via the MANUAL_TAG environment variable from workflow_dispatch).
    manual_tag = os.environ.get("MANUAL_TAG", "").strip()
    if manual_tag:
        if not manual_tag.startswith("v"):
            print(f"::error::Manual tag '{manual_tag}' must start with 'v'.", file=sys.stderr)
            set_output("skip_build", "true")
            return 1
        if not existing_release_tag(manual_tag):
            print(f"::error::Manual tag '{manual_tag}' does not exist.", file=sys.stderr)
            set_output("skip_build", "true")
            return 1
        print(f"::notice::Re-publishing existing tag {manual_tag} (manual dispatch).")
        set_output("tag", manual_tag)
        set_output("version", manual_tag)
        set_output("skip_build", "false")
        set_output("changelog_updated", "false")
        return 0

    # --- Find the merged PR --------------------------------------------------
    pr_number = find_merged_pr_number()
    if pr_number is None:
        print("::notice::No merged PR detected at HEAD; skipping release. "
              "(This typically happens for direct pushes to main.)")
        set_output("changelog_updated", "false")
        set_output("tag", "")
        set_output("version", "")
        set_output("skip_build", "true")
        return 0

    print(f"Preparing release for PR #{pr_number} ...")
    pr = gh_request("GET", f"/repos/{REPO}/pulls/{pr_number}")
    if not isinstance(pr, dict):
        print(f"::error::Unexpected PR response for #{pr_number}: {pr!r}",
              file=sys.stderr)
        return 1

    body_text = pr.get("body") or ""
    sections = parse_sections(body_text)

    # --- Determine next version ---------------------------------------------
    _prev_tag, prev_version = latest_version_tag()
    explicit = explicit_version(body_text)
    if explicit is not None:
        new_ver = explicit
        bump_used = "explicit"
    else:
        bump = determine_bump(pr)
        if bump == "explicit":
            bump = "patch"
        new_ver = next_version(prev_version, bump)
        bump_used = bump

    tag = f"v{new_ver[0]}.{new_ver[1]}.{new_ver[2]}"
    version = tag  # tag is vX.Y.Z; version string is the same

    if existing_release_tag(tag):
        print(f"::notice::Tag {tag} already exists; no new version will be created. "
              "Building and publishing against the existing tag.")
        # Still persist the release body so the publish job can pick it up.
        BODY_OUT_PATH.write_text(build_release_body(pr, sections, version), encoding="utf-8")
        # Embed (or re-embed) the release body in the annotated tag message so
        # future re-runs / manual workflow_dispatch runs can find it.
        _embed_body_in_tag(tag, pr, sections, version)
        set_output("tag", tag)
        set_output("version", version)
        set_output("skip_build", "false")
        set_output("changelog_updated", "false")
        return 0

    print(f"  previous version: v{prev_version[0]}.{prev_version[1]}.{prev_version[2]}")
    print(f"  bump:             {bump_used}")
    print(f"  next version:     {tag}")

    # --- Update CHANGELOG.md ------------------------------------------------
    entry = build_changelog_entry(version, pr, sections)
    prepend_changelog(entry)
    print("CHANGELOG.md updated.")

    # --- Prepare the release body and persist it for the publish job -------
    release_body = build_release_body(pr, sections, version)
    BODY_OUT_PATH.write_text(release_body, encoding="utf-8")

    # --- Commit and tag -----------------------------------------------------
    git("add", str(CHANGELOG_PATH))
    commit_msg = (
        f"chore(release): {tag}\n\n"
        f"Automated changelog update for {tag} (PR #{pr_number}).\n\n"
        f"{RELEASE_SKIP_MARKER}"
    )
    subprocess.run(["git", "commit", "-m", commit_msg], check=True)

    _embed_body_in_tag(tag, pr, sections, version, create=True)

    set_output("tag", tag)
    set_output("version", version)
    set_output("skip_build", "false")
    set_output("changelog_updated", "true")

    print(f"Prepared {tag} successfully.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
