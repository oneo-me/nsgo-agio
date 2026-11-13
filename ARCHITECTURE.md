# NSGo Agio architecture

Agio is the agent core split out of Neorio: drivers that run coding agent CLIs and turn their protocols into one neutral event stream, a host that folds those events into conversations, a wire protocol that carries a host to other processes, and NSGo views that show any of them. Applications compose these into a single process or a server and its clients.

## Code map

| Path | Responsibility |
| --- | --- |
| `.` (`agio`) | `Backend`, the contract between what runs conversations and what shows them, and its state types |
| `agent` | Tool-neutral driver, session, model catalog and event types, and child process plumbing shared by drivers |
| `agent/pi`, `agent/codex`, `agent/claude`, `agent/codebuddy`, `agent/acp` | One driver per CLI; `acp` runs Agent Client Protocol CLIs such as CodeBuddy Code |
| `transcript` | Turns folded from events, and the patches that carry them |
| `host` | A `Backend` running agents in this process: discovery, sessions, interruption and model choices |
| `wire` | Framed JSON peer, `Serve` for a backend and `Client`, a `Backend` mirroring one |
| `chatview` | NSGo views of any `Backend`, built from `ui/chat`, with their catalogs in `locales` |
| `shellenv`, `internal/childproc` | The login shell's environment, and process groups started and stopped as a whole |
| `examples/` | A single-process app, a server and a client |

## Boundaries

- Everything below `chatview` is headless: it never imports NSGo, so a server builds without a UI.
- `host` and `wire` speak only `agent`'s neutral types; no package above the drivers reads a CLI's own protocol.
- Views reach agents only through `agio.Backend`, so the same view shows local and remote agents. Requests run off the engine because a remote backend answers over the network.
- `Create` returns once `State` shows the conversation; views rely on it to select what they created.
- A conversation keeps the tool of its first message, as sessions cannot move between CLIs. A new message interrupts a running one instead of queueing.
- Agents run with full access in their directory and are never asked for approval; a host signals only the process groups it started.
- Agio persists nothing. An application that keeps conversations stores their `Cursor`, `Dir` and `Model` and resumes them with `Create`.

## Core flows

**Discovery.** A host reads the login environment once, then locates each CLI and reads its model catalog in parallel; each tool is usable once its own check ends.

**A message.** `Send` binds the conversation's model, appends a running turn and starts the agent when needed. Drivers may report events before `Start` returns, so the host holds them until the session is known, then applies every callback in order on one goroutine. Choices made later are applied to the live agent one at a time, latest last.

**Remote clients.** A served session publishes at most about 30 times a second: the tools and conversation headers when they change, and each conversation's turns as transcript patches, so streamed text sends only what arrived.

## Verification

Headless tests run the drivers against fake CLIs, the host and wire against a scripted driver, and `chatview` through `nsgotest`. Native windows and real agents need manual review.

## Subjects

| Document | When to read |
| --- | --- |
| [Wire protocol](docs/architecture/wire.md) | Changing how backends travel between processes |
