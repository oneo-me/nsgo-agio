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
- Add Agio, the single-process app in `cmd/agio`.
- Build Agio for macOS, Windows and Linux on arm64 and x64.
- Add automatic updates to Agio, with Check for Updates… in the macOS menu and the settings.
- Let an app place its own categories among the settings', and its actions in the window's header, with `chatview.App`.
- Show conversations as tasks: New task opens a page of its own, a hero choosing the project above a centered composer, and the sidebar lists the running tasks, then the others by when they were last active, each with its folder.
- End the sidebar with Settings and an appearance menu, and bind New task and Settings outside macOS.
- Add Agio's settings for language, theme and the shortcut modifier, keeping the computer awake while agents work, a sound and notification when an agent stops, the terminal, development tool and Git tool its title bar opens the folder in, and OpenAI-compatible model services.
- Keep Agio's window at least 820 × 740.
- Send anonymous usage statistics from Agio, with a setting to turn them off.
- Add the app icon, a prompt in a speech bubble, to Agio, the client and the website.
- Add the [website](https://oneo-me.github.io/nsgo-agio/) in English and Chinese, in light and dark appearances, with a quick start and the chat views.
