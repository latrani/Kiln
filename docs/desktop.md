# Desktop Kiln

> **AI-generated.** Claude wrote this page from Kiln's code and earlier README. If something here is wrong, please [open an issue](https://github.com/latrani/Kiln/issues).

Desktop Kiln is the same Kiln in its own window, for macOS, Windows and Linux. It's a test release: it works, but it isn't signed yet, so each system makes you say yes once. Closing the window quits (Kiln asks first if characters are connected).

It reads and writes the same worlds, characters, rules, logs and passwords as the terminal `kiln`, so the two can share a setup. [Setting up Kiln](setup.md#where-kiln-keeps-things) says where those files live. The page is the same one the [web build](web-relay.md) serves; `/backup`, `/restore` and `/reset` are unknown here, as they are in the terminal.

## Install

Grab the file for your system from the [Releases page](https://github.com/latrani/Kiln/releases). Prereleases (a version like `0.9.0-rc.1`) are listed there too.

| System | File |
|---|---|
| macOS (Apple silicon and Intel) | `Kiln-<version>-macos-universal.zip` |
| Windows, x86-64 | `Kiln-<version>-windows-amd64-installer.exe` |
| Linux, x86-64 | `Kiln-<version>-linux-amd64.AppImage`, `.deb` or `.rpm` |

### macOS

Needs macOS 13 or newer. With [Homebrew](https://brew.sh):

```sh
brew install --cask latrani/tap/kiln
```

That puts `Kiln.app` in `/Applications` and clears the quarantine flag, so it opens without a prompt.

Or download the zip, unpack it, and drag `Kiln.app` into `/Applications`. The first time you open it, macOS says it can't check the app for malware. Press **Done**, then:

1. Open **System Settings → Privacy & Security** and scroll down to the message about Kiln.
2. Press **Open Anyway** and confirm with your password.

On macOS 15 (Sequoia) and later this is the only way: Control-clicking the app and choosing Open no longer offers it. You only do this once per download.

### Windows

Run the installer. SmartScreen may say "Windows protected your PC": press **More info**, then **Run anyway**. Kiln then shows up in the Start menu.

### Linux

- **AppImage:** make it executable (`chmod +x Kiln-<version>-linux-amd64.AppImage`) and run it.
- **`.deb` or `.rpm`:** install it with your package manager (`sudo apt install ./Kiln-<version>-linux-amd64.deb`, or `sudo dnf install ./Kiln-<version>-linux-amd64.rpm`). The command is `kiln-desktop`, so it never gets in the way of the terminal client's `kiln`.

It needs GTK 4 and WebKitGTK 6.0, which the packages ask for; the AppImage is built for a recent distribution (Ubuntu 24.04 or Debian 13 and up). For saved passwords you need a Secret Service keyring, as [Setting up Kiln](setup.md#download-a-release) says.

## Things to know

**Notifications.** On macOS the status line offers **Allow notifications** until they're on; pressing it brings up the system's question about letting Kiln send them. If you refuse, Kiln says to allow them in the system's notification settings. They only work for the app when it's in an Applications folder (`/Applications` or `~/Applications`), not when it's run from Downloads. On Windows and Linux there's nothing to allow; they just work. [Notifications](notifications.md) says when Kiln sends them.

**Keychain on macOS.** Saved passwords live in your keychain. Because the app is signed only ad hoc, macOS sees every new version as a different app and asks for keychain access again after each update. Choose **Always Allow**; it goes away once Kiln has a Developer ID.

**Directories.** `XDG_CONFIG_HOME` and `XDG_DATA_HOME` ([Setting up Kiln](setup.md#where-kiln-keeps-things)) are read from the environment, and an app opened from Finder, the Start menu or a Linux launcher doesn't see variables you set only in a shell's startup file. If you've moved your folders that way, desktop Kiln uses the default ones, and not the ones your terminal `kiln` uses. Set the variables where the whole session sees them, or start it from that shell: `kiln-desktop` on Linux, and on macOS `/Applications/Kiln.app/Contents/MacOS/Kiln`, since `open -a Kiln` hands the app the system's environment, not the shell's.
