# Agio

Agio is Neorio's agent core: agent drivers, an in-process host, a wire protocol and NSGo conversation views. Read [ARCHITECTURE.md](ARCHITECTURE.md) before coding.

## Principles

- Keep the [boundaries](ARCHITECTURE.md#boundaries): headless packages never import NSGo, and views reach agents only through `agio.Backend`.
- Keep Agio product-neutral: Neorio's tasks, worktrees, Git and persistence stay in Neorio. Name the application through `agent.Client`, never in code.
- A change to driver behavior shared with Neorio is made here first; Neorio follows once it depends on this module.
- NSGo's `ui/chat` owns reusable chat components; build them there, not as wrappers here. NSGo comes from its published module pinned in `go.mod`.
- Views say their words through `chatview/locales`, in English and Simplified Chinese.

## Working with the user

- Never create sub-Agents or Git worktrees; work directly in the current checkout.
- Commit or push only when the user asks.

## Verification

Run `go tool nsgo check` and `go tool nsgo test` from the repository root, and `go tool nsgo i18n chatview/locales` after editing catalogs. Add tests for new behavior and fixed bugs. Report what native review you could not do.

## Documentation

- Update a document only when a fact its reader needs changes.
- Avoid secondary labels and descriptions that restate a heading or value.
- Write only what code and comments cannot show: purpose, boundaries and reasons.
- Keep each document within its budget; check its size after editing and trim it when over.
- Record completed user-visible outcomes as one-clause `Unreleased` bullets with references such as `(#12 by @alice)`, ordered by type. Before the first release, `Unreleased` is the first version's notes.
- Repair documents you touch instead of only appending to them.
- Keep `README.md` and `README.zh-CN.md` equivalent; everything else, including code comments, is English.
- Keep documentation on the code branch and reconcile it during merges.
