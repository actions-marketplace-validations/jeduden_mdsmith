---
title: WinGet
summary: >-
  A `jeduden.mdsmith` manifest submitted to
  `microsoft/winget-pkgs` installs mdsmith via
  `winget install jeduden.mdsmith` once the PR is
  merged.
mechanism: pull
artifact: cli
command: winget install jeduden.mdsmith
audience: Windows users with WinGet (Windows 11+)
platforms: [windows]
channelurl: https://github.com/microsoft/winget-pkgs
weight: 11
unlisted: true
---
# WinGet

Release page: <https://github.com/microsoft/winget-pkgs>

WinGet ships with Windows 11 and is available for
Windows 10 via the
[App Installer](https://apps.microsoft.com/detail/9nblggh4nns1).

Once the `jeduden.mdsmith` manifest PR is merged and
Microsoft moderation approves it, install mdsmith
with:

```powershell
winget install jeduden.mdsmith
```

Upgrade with `winget upgrade jeduden.mdsmith`.

The `winget-submit` job in `release.yml` runs
`komac` on each release. komac builds the manifest
from the published Windows binary URL and opens the
PR. It authenticates with the `WINGET_PR_TOKEN` repo
secret, gated by the `release` environment. A
missing token, or any komac failure, logs a notice
and skips the step. The release never fails.

The first WinGet version is bootstrapped by hand.
`mdsmith-release render-winget-manifest` emits the
three manifest files — version, installer, and
locale — from `checksums.txt`. The installer
manifest declares `InstallerType: portable` with
`PortableCommandAlias: mdsmith`, because the asset is
a standalone binary, not an installer: WinGet stores
it and links it onto PATH as `mdsmith` rather than
executing it. This mirrors how `render-scoop-manifest`
bootstraps the Scoop bucket. After that,
`komac update` keeps each release current. Workflow
logic stays in `mdsmith-release` and `komac`, not
inline shell, per the
[release-tooling rule](../release-tooling.md).

The short `winget install jeduden.mdsmith` form works
only after the initial manifest PR merges and
Microsoft's moderation queue processes it. Until
then, the GitHub release `.exe` is the documented
fallback.

Because nothing installs through WinGet yet, this
channel sets `unlisted: true` in its frontmatter, so
`sync-channels` keeps it out of the website install
picker and the install-guide table excludes it by
glob. The tooling and this doc stay; only the
user-facing listings wait for the manifest PR to
land. Drop both once `winget install jeduden.mdsmith`
resolves.
