# Changelog

## [Unreleased]

- Let apps add pages of their own after New task in the sidebar with `App.Pages`.
- Let apps append to agents' system prompts with `host.Options.SystemPrompt` and `agio.Create.SystemPrompt`.
- Let apps attach skills to agents with `host.Options.Skills` and `agio.Create.Skills`.
- Let apps attach MCP servers to agents with `host.Options.MCPServers` and `agio.Create.MCPServers`.
- Let apps serve their own tools to agents with `mcp`, an MCP server over streamable HTTP on the loopback interface behind a token, and tell their agents apart with `Server.AgentFor` and `mcp.Caller`.
- Let apps with a window of their own compose chatview's parts: `Quotas`, `SettingsDialog`, `Footer`, `AppMenu` and `Composer`, and a `Conversation` showing some of its turns or something in place of its composer.
- Let apps with settings of their own show the agents' with `chatview.Agents`, and hear of messages sent with `Conversation.OnSend`.
- Let apps title conversations with an OpenAI-compatible model at a reasoning effort with `naming`, and set the effort of Agio's naming model in its settings.
- Show each account's limit at the sidebar's end, and the context window at the composer's end.
- Fix conversations resumed right after launch failing as their agent being unavailable while it is still looked for.

## [0.1.0] - 2026-11-13

Agio 0.1.0 is the first release of Neorio's agent core as a Go library, and of Agio, the app built on it, for macOS, Windows and Linux.

- Add drivers for pi, Codex, Claude Code and CodeBuddy Code behind one neutral event stream.
- Add `host.Host` to run conversations in this process, and `wire` to serve them to clients over TCP or any byte stream.
- Add `chatview.App` and `chatview.Conversation` to show any backend with NSGo, with tasks, agent and model settings, and account limits.
- Add Agio, the app in `cmd/agio`, for macOS, Windows and Linux on arm64 and x64, with automatic updates.
- Add tasks in projects to Agio, resumed after a restart and titled by a naming model.
- Add color schemes, background grain, notifications and OpenAI-compatible model services to Agio's settings.
- Send anonymous usage statistics from Agio, with a setting to turn them off.
