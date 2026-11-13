<h1 align="center">Agio</h1>

<p align="center">
  <strong>English</strong> |
  <a href="README.zh-CN.md">简体中文</a>
</p>

<p align="center">
  <a href="https://oneo-me.github.io/nsgo-agio/"><img alt="Website" src="https://img.shields.io/badge/Website-oneo--me.github.io-0969da?logo=astro&logoColor=white"></a>
  <a href="https://github.com/oneo-me"><img alt="GitHub" src="https://img.shields.io/badge/GitHub-oneo--me-181717?logo=github&logoColor=white"></a>
  <a href="https://oneo.me"><img alt="Blog" src="https://img.shields.io/badge/Blog-oneo.me-0969da"></a>
  <a href="https://x.com/oneo666"><img alt="X" src="https://img.shields.io/badge/X-%40oneo666-000000?logo=x&logoColor=white"></a>
  <a href="https://github.com/sponsors/oneo-me"><img alt="GitHub Sponsors" src="https://img.shields.io/badge/GitHub%20Sponsors-ea4aaa?logo=githubsponsors&logoColor=white"></a>
</p>

<p align="center">
  The agent core of Neorio: run coding agents (pi, Codex, Claude Code, CodeBuddy Code) and show their conversations with NSGo, in one process or as a server and its clients.
</p>

## Install

```sh
go get github.com/oneo-me/nsgo-agio
```

It needs Go 1.27 and the agent CLIs you want on the login shell's `PATH`; it runs them with your own sign-ins.

## Usage

An application chooses where its agents run by the backend it shows. In one process, a `host.Host` runs them:

```go
h := host.New(host.Options{Dir: workDir})
nsgo.Run(func(cx *core.Scope) core.Node {
	return chatview.App{Model: chatview.Bind(cx, h)}
})
h.Close()
```

As a server and its clients, the server serves its host and a client shows a `wire.Client` instead:

```go
// Server
wire.ListenAndServe(host.New(host.Options{Dir: workDir}), "127.0.0.1:7788", token)

// Client
c, err := wire.Dial("127.0.0.1:7788", token)
nsgo.Run(func(cx *core.Scope) core.Node {
	return chatview.App{Model: chatview.Bind(cx, c)}
})
```

`chatview.Conversation` shows a single conversation for an application with its own layout, and `wire.Serve` serves any byte stream, such as an SSH session. Agents run with full access to the server's files, so keep a server on a private network and give it a token.

Agio, the app in `cmd/agio`, runs its agents in one process; the examples run as a server and its client:

| Program | Run |
| --- | --- |
| Agio | `go tool nsgo dev cmd/agio` |
| Server | `go run ./examples/server -token secret` |
| Client | `AGIO_TOKEN=secret go tool nsgo dev examples/client` |

Agio sends anonymous usage statistics to Umami: when it is open and used, and which agents messages go to, never prompts or folders. Builds run with `nsgo dev` send nothing, and its settings turn them off.

`go tool nsgo build cmd/agio` builds Agio for macOS, Windows and Linux, on arm64 and x64. Releases update themselves from the latest GitHub release when built with `NSGO_UPDATES_KEY`, the secret key of `updates.key` in `cmd/agio/nsgo.yaml`.

## Documentation

[ARCHITECTURE.md](ARCHITECTURE.md) maps the packages and the boundaries between them.

## Development

Run `go tool nsgo check` and `go tool nsgo test` from the repository root. See [AGENTS.md](AGENTS.md) for how changes are made. The website is in [website/](website/README.md).

See [CHANGELOG.md](CHANGELOG.md) for release notes.

## License

MIT, see [LICENSE](LICENSE).
