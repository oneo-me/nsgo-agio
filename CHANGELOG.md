# Changelog

## [Unreleased]

- Let apps add pages of their own after New task in the sidebar with `App.Pages`.
- Show each account's limit at the sidebar's end, and the context window at the composer's end.

## [0.1.0] - 2026-11-13

Agio 0.1.0 is the first release of Neorio's agent core as a Go library, and of Agio, the app built on it, for macOS, Windows and Linux.

- Add drivers for pi, Codex, Claude Code and CodeBuddy Code behind one neutral event stream.
- Add `host.Host` to run conversations in this process, and `wire` to serve them to clients over TCP or any byte stream.
- Add `chatview.App` and `chatview.Conversation` to show any backend with NSGo, with tasks, agent and model settings, and account limits.
- Add Agio, the app in `cmd/agio`, for macOS, Windows and Linux on arm64 and x64, with automatic updates.
- Add tasks in projects to Agio, resumed after a restart and titled by a naming model.
- Add color schemes, background grain, notifications and OpenAI-compatible model services to Agio's settings.
- Send anonymous usage statistics from Agio, with a setting to turn them off.
