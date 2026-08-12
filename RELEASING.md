# Releasing ADBPureFlow

Releases are produced **automatically** by the GitHub Actions workflow defined
in `.github/workflows/release.yml`. There are no manual release steps in the
usual case. This document describes how the automation works and how to
recover from failures.

## Lifecycle

1. **Development branch.** Every push to a development branch (including the
   `arena/*` namespace used by Arena.ai sessions, feature branches, personal
   forks, etc.) runs the read-only `CI` workflow: `gofmt` check, `go vet`,
   unit tests, and a multi-platform build smoke test. These jobs **never**
   publish artifacts, create Git tags, or create GitHub Releases — the CI
   workflow is pinned to `contents: read`.

2. **Pull request → `main`.** When you open a PR targeting `main`, the same
   `CI` workflow runs against the PR. Fill in the PR template sections:
   - `## Summary` — human-readable description of the change (becomes the
     **main body** of the GitHub Release).
   - `## Validation` — how the change was tested (becomes the Validation
     section of the release notes).
   - `## Breaking Changes` — describe any breaking changes, or leave as
     `None.`
   - `## Notes` — optional additional context.

   Select the semantic version impact via the **MAJOR / MINOR / PATCH**
   checkbox under `## Type of Change`, or apply one of the labels
   `breaking`, `feature`/`enhancement`, `bug`/`fix`, `chore`, `ci`, `docs`,
   `documentation`, `maintenance`. If nothing is selected, the release
   defaults to a **PATCH** bump. You may also force a specific version by
   adding `Release-As: vX.Y.Z` anywhere in the PR body, or
   `Semver: major|minor|patch`.

3. **Merge.** When the PR is merged into `main`, the `Release` workflow fires
   on the push-to-main event. The `version-bump` job does the following
   inside a single workflow run:
   - Identifies the merged PR from the merge commit (both standard merge
     commits and squash merges are recognized).
   - Computes the next [SemVer](https://semver.org) based on the labels,
     checkboxes, and `Release-As`/`Semver` directives.
   - Extracts the `Summary`, `Validation`, `Breaking Changes`, and `Notes`
     sections from the PR body.
   - Prepends a new entry to `CHANGELOG.md` (history is always preserved).
   - Commits the CHANGELOG update with the marker `[release skip]` in the
     commit message so the automation won't try to produce another release
     from the commit it just created.
   - Creates an annotated tag `vX.Y.Z` with the rendered release notes
     embedded between `---RELEASE-BODY-START---` and `---RELEASE-BODY-END---`
     markers inside the tag message.
   - Pushes the commit and tag to `origin/main`. GitHub does **not** re-
     trigger workflows from token-pushed events (loop prevention), so the
     build and publish continue in the **same** run.

4. **Build + publish.** The `build` job checks out the new tag and compiles
   the CLI and GUI for the existing platform matrix (Windows, Linux, macOS,
   all amd64), stamping `main.version` via `-ldflags`. The `publish-release`
   job then collates the binaries and creates (or edits) the GitHub Release,
   attaching all artifacts and using the release notes prepared in step 3.
   Artifact names keep the existing convention
   `adbpureflow-{cli,gui}-{goos}-{goarch}[.exe]`.

## Branch permissions

| Branch / ref                | Lint / Test / Build | Create tag | Publish Release |
|-----------------------------|:-------------------:|:----------:|:---------------:|
| `arena/*` and other branches | ✅                  | ❌         | ❌              |
| Pull requests → `main`      | ✅                  | ❌         | ❌              |
| Direct push to `main`*      | ✅ (CI)             | ❌         | ❌              |
| Release run on `main` (PR merge) | ✅              | ✅         | ✅              |
| `workflow_dispatch` (existing tag) | ✅             | ❌         | ✅              |

> \* A direct push to `main` that is not a PR merge (no PR merge commit
> detected at HEAD) will **not** create a release; `version-bump` logs a
> notice and exits. Always land changes through a PR.

## Safety and idempotency

The pipeline is designed to be safe to rerun and free of release loops:

- The CHANGELOG commit contains `[release skip]`, which the version-bump job
  detects at HEAD and no-ops on → no loop.
- If the tag already exists (e.g. from a partially-failed run), the
  version-bump job skips the commit/tag steps and proceeds directly to
  build + publish against the existing tag.
- `softprops/action-gh-release` edits an existing release in place, so
  re-running `publish-release` overwrites the body and adds/replaces assets
  rather than failing.
- Build is always performed against the tag (not against the branch tip), so
  binaries reflect exactly the commit that was released.

## Recovering from a failed release

- If the `version-bump` job failed before pushing anything, simply re-run
  the failed job from the Actions UI.
- If the CHANGELOG commit and/or tag was pushed but the build failed, re-run
  the failed `build` job (it checks out the tag directly). You can also
  re-run the entire `Release` workflow from the Actions UI on a completed
  run, or trigger it manually from the **Actions → Release → Run workflow**
  button, passing the existing tag in the `tag` input — this will rebuild
  and re-publish binaries without bumping the version.
- If you need to roll back, delete the tag and the `chore(release)` commit
  on `main` and revert the merge PR; then open a new PR with the fix and
  merge it as usual.

## Local verification

You can exercise the release-notes parser locally without pushing anything:

```bash
# Syntax check the helper scripts
python .github/scripts/prepare_release.py
python .github/scripts/build_release_body.py
```

The scripts depend only on the Python standard library — no third-party
packages are required, which keeps the release pipeline deterministic and
secure.
