<!--
  Thank you for contributing to ADBPureFlow! 🎉

  PLEASE READ BEFORE SUBMITTING:
  * Fill in each section below. Sections left empty will be omitted from the
    automatically generated release notes, but the section headings should be
    left in place so the release tooling can parse your PR.
  * Select the Semantic Versioning impact of your change using the checkbox
    under "Type of Change". This determines whether your change produces a
    MAJOR, MINOR, or PATCH release when merged into `main`. If you do not
    select anything, the release will default to a PATCH bump.
  * Keep the `## Summary`, `## Validation`, `## Breaking Changes`, and
    `## Notes` headings exactly as written (case-insensitive, `##` prefix);
    the release automation parses them directly.
  * Optional: add a line `Release-As: vX.Y.Z` anywhere in this description to
    force a specific version number (overrides automatic bump logic).
-->

## Summary

<!--
  A concise, human-readable description of what this PR does and why.
  This becomes the MAIN BODY of the GitHub Release when the PR is merged.
  Use Markdown freely: lists, code blocks, links, etc. are preserved.
-->

- Replace this bullet with a summary of your change.
- Explain *what* changed and *why* it matters to users or contributors.

Fixes # (issue)

## Validation

<!--
  How did you verify this change? Test commands, platforms tested, manual
  reproduction steps, screenshots, or evidence. This becomes the "Validation"
  section of the release notes so users/developers can see how the change was
  tested.
-->

- [ ] `gofmt -w CLI GUI` (or `gofmt -l CLI GUI` reports no files)
- [ ] `cd CLI && go vet ./... && go test -v -race ./...`
- [ ] `cd GUI && go vet ./... && go test -v -race ./...`
- [ ] Manually tested on the affected platform(s) (describe below):

<!-- Add any additional validation details here. -->

## Breaking Changes

<!--
  If this PR introduces any BREAKING CHANGE (CLI flags, GUI behavior,
  on-disk layout, API, required Go version, etc.), describe it here and
  check the "Breaking change" box below. If there are no breaking changes,
  leave this section as `None.`.
-->

None.

## Notes

<!--
  Anything else reviewers and users should know: follow-up work, known
  limitations, migration steps, deprecations, credits, links to related
  issues or designs. Remove this section if unused.
-->

<!-- SEPARATOR -->

## Type of Change

<!--
  Check exactly ONE box. This controls the Semantic Version bump when the PR
  is merged. If no box is checked, the release defaults to a PATCH bump.
  You may also indicate a bump via labels on the PR (`breaking`, `feature`,
  `bug`, `chore`, `ci`, `docs`, ...).
-->

- [ ] **MAJOR** — Breaking change (existing behavior/API changes; users may need to migrate)
- [ ] **MINOR** — New feature (backward-compatible; adds functionality)
- [ ] **PATCH** — Bug fix, chore, docs, CI, or other maintenance change
