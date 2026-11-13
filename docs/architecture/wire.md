# Wire protocol

`wire` carries an `agio.Backend` between processes so that a client is itself a `Backend`, and views never know where agents run.

## Frames

A connection is one byte stream of length-prefixed frames, each a JSON request, response or event. Any stream works, so a server listens on TCP, runs under an SSH session or speaks over a pipe in tests. Requests are answered in arrival order by the backend, which never waits for an agent, so nothing queues behind a slow agent.

## Sessions

A client's first request is Hello with the protocol version and the server's token. A server of another version answers with its own and serves nothing else, so mismatched builds fail clearly instead of misreading each other. A token is compared in constant time; without one, a server is for this computer alone, as a client controls agents with full access to the server's files.

## Publications

The server never sends the whole state twice. Each session remembers what it sent and publishes, at most about 30 times a second, the tools and conversation headers when they changed, and each conversation's turns as a transcript patch: new and changed segments, with streamed text appended. A new session starts with a reset. Coalescing keeps a streamed reply from flooding a slow link while still reading as live.

Requests are answered before the publication they cause, so a client's `Create` waits for the publication showing its conversation, keeping the `Backend` promise that `State` shows what `Create` returned. A closed connection leaves the server's agents running; the client reports the loss through `State.Err`.
