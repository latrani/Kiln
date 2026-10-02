# Notifications

> **AI-generated.** Claude wrote this page from Kiln's code and earlier README. If something here is wrong, please [open an issue](https://github.com/latrani/Kiln/issues).

When you're away from Kiln, activity shows up as a desktop notification like `Kit: Rook pages: you around?` (`Kit@fm:` when two worlds have a Kit). You're away when you switch to another window or tab, after `notify_idle` (default 5 minutes) without typing or clicking, or from `/away` until your next key or click. Only what arrives while you're away notifies; a line that came while you were here, you saw, so it's never sent later. The `notify` setting picks what notifies: `first` (the default) sends the first line since you left and then only lines that need attention (pages and whispers), `all` sends every line, `attention` only those, and `none` nothing. Quiet lines never notify, and for a few seconds after connecting only attention lines do, so the login banner stays quiet. A burst of lines (like a room description) notifies only its first line. `/notify` changes it for one character until Kiln quits.

Notifications work in iTerm2, kitty, Ghostty, WezTerm, foot and Blink, locally or over ssh. Inside tmux, add this to `~/.tmux.conf`:

    set -g allow-passthrough on
    set -g focus-events on

Then run `tmux source-file ~/.tmux.conf` and detach and reattach: tmux asks your terminal for focus events only when you attach.

The top bar shows what Kiln thinks about you: `● here`, `○ away`, or `? focus`. `? focus` means it has never heard a focus change from your terminal, which is either because you haven't switched windows yet (terminals only report when focus changes) or because your terminal doesn't report it at all (Blink on iOS, mosh, tmux without `focus-events on`). Switch windows once to find out: if it then says `○ away` and back to `● here`, focus works. If it doesn't, click the chip to set Away yourself, as `/away` does, and notifications go out until your next key or click. Idle time still counts as away either way. Clicking the chip while away ends it.

Mosh drops notifications, but it passes on the bell: set `notify_method = "both"` and turn on Blink's "Notification on background shell" to get an alert (without the line).

## Over ssh

Copying and links work through your terminal, not the remote machine: Kiln writes the text to your clipboard with OSC 52, which goes back down the ssh connection to the terminal you're looking at. A click on a link can't open a browser on your screen, so over ssh (Kiln checks `SSH_CONNECTION`) it copies the link instead, and does the same anywhere the system's opener fails.

Your terminal has to allow it. iTerm2: Settings → General → Selection → "Applications in terminal may access clipboard". kitty, Ghostty, WezTerm and Blink allow it by default. Inside tmux, the `allow-passthrough on` above is what lets it through (`set -g set-clipboard on` also works).
