---
title: WINGET_PR_TOKEN
summary: >-
  GitHub PAT for forking microsoft/winget-pkgs and
  opening a manifest PR via komac. Gated by the
  `release` environment.
lastRotated: "2026-06-05"
periodDays: 335
provider: GitHub
issuerUrl: "https://github.com/settings/personal-access-tokens"
usedBy: "release.yml (winget-submit)"
scope: "Contents: read+write; Pull requests: read+write (fork of microsoft/winget-pkgs)"
releaseEnvScoped: true
---
# WINGET_PR_TOKEN

Generated at the
[GitHub fine-grained tokens page][gh-pat]. The
`winget-submit` job needs permission to fork
`microsoft/winget-pkgs` (if not already forked) and
open a pull request from the fork.

Settings on issuance:

- **Resource owner:** jeduden.
- **Repository access:** All repositories (a fork of
  microsoft/winget-pkgs will be created under the
  token owner's account on first submission).
- **Repository permissions:**
  - Contents: Read and write
  - Pull requests: Read and write
  - Metadata: Read (automatic)
- **Expiration:** 1 year.

Store the value as the `WINGET_PR_TOKEN` secret on
the `release` GitHub environment.
[Environment settings page.][env-settings]
The `winget-submit` job is best-effort: a missing or
expired token logs a notice and skips the submission
without failing the release.

[gh-pat]: https://github.com/settings/personal-access-tokens
[env-settings]: https://github.com/jeduden/mdsmith/settings/environments
