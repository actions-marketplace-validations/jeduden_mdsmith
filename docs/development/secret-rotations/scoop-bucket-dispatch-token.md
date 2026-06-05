---
title: SCOOP_BUCKET_DISPATCH_TOKEN
summary: >-
  GitHub fine-grained PAT for dispatching a manifest
  bump to the jeduden/scoop-mdsmith bucket. Plain repo
  secret — not gated by an environment.
lastRotated: "2026-06-05"
periodDays: 335
provider: GitHub
issuerUrl: "https://github.com/settings/personal-access-tokens"
usedBy: "release.yml (notify-scoop-bucket)"
scope: "Contents: read+write (jeduden/scoop-mdsmith only)"
releaseEnvScoped: false
---
# SCOOP_BUCKET_DISPATCH_TOKEN

Generated at the
[GitHub fine-grained tokens page][gh-pat]. This token
is **not** environment-scoped. The bucket at
`jeduden/scoop-mdsmith` self-bumps daily via its
`checkver` schedule, so a missing token only delays
an immediate bump. The next scheduled run covers it.

Settings on issuance:

- **Resource owner:** jeduden.
- **Repository access:** Only select repositories →
  `jeduden/scoop-mdsmith`.
- **Repository permissions:**
  - Contents: Read and write
  - Metadata: Read (automatic)
- **Expiration:** 1 year.

Store as the `SCOOP_BUCKET_DISPATCH_TOKEN` repo
secret on the
[Actions secrets page][actions-secrets]. The
reminder workflow opens an issue 30 days before
expiry; rotate then to keep dispatches immediate.

[gh-pat]: https://github.com/settings/personal-access-tokens
[actions-secrets]: https://github.com/jeduden/mdsmith/settings/secrets/actions
