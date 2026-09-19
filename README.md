# Ctrl+Shift+3 CLI (cs3)

Terminal client for [Ctrl+Shift+3](https://ctrlshift3.com). Catch up on the feed, search, read comments, check your wall and pings, change account settings, and post plain-text thoughts.

## Install

Mac / Linux:

```bash
curl -fsSL https://ctrlshift3.com/CLI/install.sh | bash
```

Windows (PowerShell):

```powershell
irm https://ctrlshift3.com/CLI/install.ps1 | iex
```

Or grab a zip from [Varieties](https://ctrlshift3.com/varieties), unzip, and open the launcher. Log in with your site username (or email) and password.

Run `cs3` with no arguments for the interactive menu. `cs3 install-path` copies the binary somewhere you can put on your PATH.

## Build from source

```bash
go build -o cs3 ./cmd/cs3
```

Site release zips: `make release` (output under `dist/`).

## Trust notes

- Talks to `https://ctrlshift3.com` over HTTPS.
- API token lives in the OS keyring, not in this repo.
- No art or poll compose. No analytics or telemetry.
- Same account as the website and iOS app.

## Commands

- `cs3` - interactive menu
- `cs3 login` / `cs3 logout` - sign in or out (logout ends the CLI session on the server)
- `cs3 whoami` - signed-in username
- `cs3 feed` - latest main-feed posts; pick a number to read and comment
- `cs3 post <id>` - one post and its comments
- `cs3 post create` - plain-text thought (end the body with a line that is only `.`)
- `cs3 mine` - your posts; view, edit, or delete when allowed
- `cs3 post edit <id>` / `cs3 post delete <id>` - edit or delete a thought
- `cs3 wall` - your profile wall
- `cs3 quote @user` - someone else's wall (when you see `"N` after their name)
- `cs3 pings` / `cs3 pings past` - inbox and opened pings
- `cs3 search ...` - global search (posts, people, Clicks)
- `cs3 click search [slug] [query...]` - search posts inside one Click you belong to
- `cs3 settings` / `cs3 settings get` / `cs3 settings set ...` - account prefs
- `cs3 install-path` - install a copy for PATH

Comments are multi-line. Finish with a line containing only `.`. Skip with `b` then `.` where prompted.

## License

Copyright (C) 2026 Billy Wilcosky

This program is free software: you can redistribute it and/or modify it under the terms of the GNU General Public License as published by the Free Software Foundation, either version 3 of the License, or (at your option) any later version.

See [LICENSE](LICENSE) for the full GPLv3 text.
