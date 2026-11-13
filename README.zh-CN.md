<h1 align="center">Agio</h1>

<p align="center">
  <strong>简体中文</strong> |
  <a href="README.md">English</a>
</p>

<p align="center">
  <a href="https://oneo-me.github.io/nsgo-agio/zh/"><img alt="官网" src="https://img.shields.io/badge/%E5%AE%98%E7%BD%91-oneo--me.github.io-0969da?logo=astro&logoColor=white"></a>
  <a href="https://github.com/oneo-me"><img alt="GitHub" src="https://img.shields.io/badge/GitHub-oneo--me-181717?logo=github&logoColor=white"></a>
  <a href="https://oneo.me"><img alt="博客" src="https://img.shields.io/badge/%E5%8D%9A%E5%AE%A2-oneo.me-0969da"></a>
  <a href="https://x.com/oneo666"><img alt="X" src="https://img.shields.io/badge/X-%40oneo666-000000?logo=x&logoColor=white"></a>
  <a href="https://github.com/sponsors/oneo-me"><img alt="GitHub 赞助" src="https://img.shields.io/badge/GitHub%20%E8%B5%9E%E5%8A%A9-ea4aaa?logo=githubsponsors&logoColor=white"></a>
</p>

<p align="center">
  Neorio 的智能体核心：运行编程智能体（pi、Codex、Claude Code、CodeBuddy Code），并用 NSGo 展示对话，可在单个进程中运行，也可拆成服务端与客户端。
</p>

## 安装

```sh
go get github.com/oneo-me/nsgo-agio
```

需要 Go 1.27，以及登录 shell 的 `PATH` 中你想使用的智能体 CLI；它们使用你自己的登录状态运行。

## 使用

应用通过它展示的后端决定智能体在哪里运行。单进程时由 `host.Host` 运行：

```go
h := host.New(host.Options{Dir: workDir})
nsgo.Run(func(cx *core.Scope) core.Node {
	return chatview.App{Model: chatview.Bind(cx, h)}
})
h.Close()
```

服务端加客户端时，服务端提供它的 host，客户端改为展示 `wire.Client`：

```go
// 服务端
wire.ListenAndServe(host.New(host.Options{Dir: workDir}), "127.0.0.1:7788", token)

// 客户端
c, err := wire.Dial("127.0.0.1:7788", token)
nsgo.Run(func(cx *core.Scope) core.Node {
	return chatview.App{Model: chatview.Bind(cx, c)}
})
```

自有布局的应用可用 `chatview.Conversation` 展示单个对话；`wire.Serve` 可在任意字节流上提供服务，例如 SSH 会话。智能体对服务端文件拥有完全访问权限，因此请把服务端放在私有网络中并设置令牌。

`cmd/agio` 中的应用 Agio 在单进程中运行智能体；示例以服务端和客户端运行：

| 程序 | 运行 |
| --- | --- |
| Agio | `go tool nsgo dev cmd/agio` |
| 服务端 | `go run ./examples/server -token secret` |
| 客户端 | `AGIO_TOKEN=secret go tool nsgo dev examples/client` |

Agio 会向 Umami 发送匿名使用统计：何时打开和使用，以及消息发给了哪些智能体，从不包含提示词或文件夹。`nsgo dev` 运行的构建不发送，也可在设置中关闭。

`go tool nsgo build cmd/agio` 为 macOS、Windows 和 Linux 构建 arm64 与 x64 的 Agio。构建时设置 `NSGO_UPDATES_KEY`（`cmd/agio/nsgo.yaml` 中 `updates.key` 对应的私钥），发布的版本就会从最新的 GitHub Release 自动更新。

## 文档

[ARCHITECTURE.md](ARCHITECTURE.md) 说明各个包及它们之间的边界。

## 开发

在仓库根目录运行 `go tool nsgo check` 和 `go tool nsgo test`。修改方式见 [AGENTS.md](AGENTS.md)。官网位于 [website/](website/README.md)。

发布说明见 [CHANGELOG.md](CHANGELOG.md)。

## 许可证

MIT，见 [LICENSE](LICENSE)。
