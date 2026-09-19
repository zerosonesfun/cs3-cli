Ctrl+Shift+3 CLI (cs3)
======================

A terminal client for the site: feed, search, comments, your wall, pings, account
settings, and plain-text posting — without opening a browser.

Recommended (Mac / Linux)
-------------------------
Open Terminal and run:

  curl -fsSL https://ctrlshift3.com/CLI/install.sh | bash

That downloads the right build, installs to ~/bin/cs3, clears macOS quarantine
when needed, and starts the app. Next time you can run: cs3

If ~/bin is not on your PATH, the installer prints a one-line fix.

Recommended (Windows)
---------------------
Open PowerShell and run:

  irm https://ctrlshift3.com/CLI/install.ps1 | iex

That installs to %LOCALAPPDATA%\cs3\cs3.exe and starts the app.

Zip / launcher (this folder)
----------------------------
1. Unzip this folder somewhere convenient.
2. Open the launcher:
   - macOS: double-click "Open CtrlShift3 CLI.command"
     (If macOS blocks it: prefer the curl installer above, or right-click →
     Open, or: xattr -dr com.apple.quarantine .)
   - Windows: double-click "Open CtrlShift3 CLI.cmd"
   - Linux: run ./open-cs3.sh (or: chmod +x cs3 && ./cs3)
3. Choose Log in and use your site username (or email) and password.

Optional: run "cs3 install-path" so you can type "cs3" from any terminal
(~/bin on Mac/Linux, or %LOCALAPPDATA%\cs3 on Windows — add that folder to PATH).

Commands
--------

cs3
  Interactive welcome menu (same as opening the launcher).
  Pick Help (2, h, or ?) for a short cheat sheet. Latest feed is
  numbered: type a post number to read and comment; b goes back.
  Example: browse feed and pings with number keys when you do not want
  to remember command names.

cs3 login / cs3 logout
  Sign in or sign out. Logout ends the CLI session; Quit only closes the
  app and leaves you signed in for next time.
  Example: on a shared computer, run "cs3 logout" before you leave.

cs3 whoami
  Print the username you are signed in as.
  Example: confirm you are on the right account before changing settings.

cs3 feed
  Latest 20 main-feed posts as a numbered list. Type a post number to
  open the full post and comments, then comment (end with . ; b then .
  skips). Type quote @user to open their wall when you see "N after
  their name. Type b to go back.
  Example: morning catch-up — skim the list, type 3 to read #3 and reply.

cs3 post <id>
  Show one post and its comments, then prompt to comment (use the id
  from the feed list).
  Example: cs3 post a1b2c3d4

cs3 post create
  Compose a plain-text thought (main feed, your wall, or a Click).
  End the body with a line containing only a period (.).
  Art and polls are not supported from the CLI.
  Example: cs3 post create

cs3 mine
  List your posts; pick a number to view, edit, or delete when allowed.
  Example: cs3 mine

cs3 post edit <id>
cs3 post delete <id>
  Edit or delete one of your thought posts.
  Examples:
    cs3 post edit a1b2c3d4
    cs3 post delete a1b2c3d4

cs3 wall
  Posts on your profile wall.
  Example: see what people left on your profile while you were away.

cs3 quote @username
  Open someone else's profile wall when you see "N (or "10+) after
  @user in a feed or thread. Clears that unread wall signal.
  Example: cs3 quote alice
  From the feed menu you can also type: quote @alice

cs3 pings
  Your ping inbox, with * for new since last CLI check. Type a number
  to open a ping (moves it to Past). Related comment pings for the same
  post can be dismissed together if you confirm.
  Example: check mentions and comment pings, then open one.

cs3 pings past
  Opened (past) pings, 20 per page with Load more. Type a number to open
  again, or c to delete all past after confirmation.
  Example: browse older pings, then clear the archive.

cs3 search
  Search posts, people, and Clicks (same as the website). Query must be
  at least 2 characters; omit the query to be prompted. After post hits,
  type a number to open the thread and comment.
  Example: cs3 search coffee

cs3 settings
cs3 settings get
cs3 settings set ...
  View or change account prefs (theme, font, sounds, profile visitors,
  Click invite blocking, @mention / post-comment / reply pings, email digests,
  hide bubbled posts, Fediverse, pet name).
  Examples:
    cs3 settings
    cs3 settings get
    cs3 settings set --theme dark --sound on
    cs3 settings set --hide-bubbles on
    cs3 settings set --federate off
    cs3 settings set --pet-name "Mochi"
    cs3 settings set --username NewName

cs3 install-path
  Copy this app into a folder you can put on your PATH.
  Example: after install-path, open a new terminal and run "cs3 feed".
