# Changelog

## [Unreleased]

- Add drivers for pi, Codex, Claude Code and CodeBuddy Code behind one neutral event stream.
- Add `host.Host` to run conversations in this process.
- Add `wire` to serve a backend to clients over TCP or any byte stream.
- Add `chatview.App` and `chatview.Conversation` to show any backend with NSGo.
- Add settings to turn agents and models on and off.
- Show account limits in the composer and beside the model picker's groups.
- Show tool and provider marks beside models.
- Add the macOS menu bar with Settings and New conversation.
- Let a new conversation choose its project folder before its first message.
- Add Agio, the single-process app in `cmd/agio`.
- Build Agio for macOS, Windows and Linux on arm64 and x64.
- Add automatic updates to Agio, with Check for Updates… in the macOS menu and the settings.
- Let an app add its own categories to the settings with `chatview.App.Settings`.
- Send anonymous usage statistics from Agio, with a setting to turn them off.
- Add the app icon, a prompt in a speech bubble, to Agio, the client and the website.
- Add the [website](https://oneo-me.github.io/nsgo-agio/) in English and Chinese, in light and dark appearances, with a quick start and the chat views.
