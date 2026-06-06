---
title: 'string & != ""'
summary: 'string & != ""'
mechanism: '"push" | "pull" | "toolchain"'
artifact: '"cli" | "vscode-extension" | "claude-plugin" | "obsidian-plugin"'
command: 'string & != ""'
command-windows: 'string | *""'
audience: 'string & != ""'
platforms: '[...string] | *[]'
registry: '[if mechanism == "push" {string & != ""}, (string | *"")][0]'
credential: '[if mechanism == "push" {string & != ""}, (string | *"")][0]'
job: '[if mechanism == "push" {string & != ""}, (string | *"")][0]'
channelurl: 'string & =~ "^https?://"'
weight: 'int & >=1'
unlisted: 'bool | *false'
---
# {title}

Release page: <{channelurl}>

## ...

<?allow-empty-section?>
