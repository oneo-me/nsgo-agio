package wire

import (
	"net"
	"testing"
	"time"

	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/host"
	"github.com/oneo-me/nsgo-agio/internal/fakeagent"
	"github.com/oneo-me/nsgo-agio/transcript"
)

// connect serves a host with the fake agent to a client over a pipe.
func connect(t *testing.T, token string) (*host.Host, *Client, error) {
	t.Helper()
	h := host.New(host.Options{Env: []string{}, Tools: []agent.Tool{agent.Claude},
		Drivers: map[agent.Tool]agent.Driver{agent.Claude: &fakeagent.Driver{}}, Home: t.TempDir()})
	t.Cleanup(func() { h.Close() })
	server, conn := net.Pipe()
	go Serve(h, server, "secret")
	c, err := Connect(conn, token)
	if err == nil {
		t.Cleanup(func() { c.Close() })
	}
	return h, c, err
}

func waitFor(t *testing.T, b agio.Backend, ok func(agio.State) bool) agio.State {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s := b.State(); ok(s) {
			return s
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("state never matched: %+v", b.State())
	return agio.State{}
}

func TestClientMirrorsTheHost(t *testing.T) {
	_, c, err := connect(t, "secret")
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, c, func(s agio.State) bool { return s.Ready() })
	id, err := c.Create(agio.Create{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if c.State().Conversation(id) == nil {
		t.Fatal("created conversation not shown yet")
	}
	if err := c.Send(id, agio.Message{Text: "over the wire"}); err != nil {
		t.Fatal(err)
	}
	s := waitFor(t, c, func(s agio.State) bool {
		conv := s.Conversation(id)
		return conv != nil && len(conv.Turns) == 1 && conv.Turns[0].Status == transcript.Completed
	})
	conv := s.Conversation(id)
	if conv.Turns[0].Final().Text != "echo: over the wire" || conv.Title != "over the wire" || conv.Model != "claude:opus" {
		t.Fatalf("mirrored %+v, %+v", conv, conv.Turns[0])
	}
	haiku := "claude:haiku"
	if err := c.Choose(id, agio.Choice{Model: &haiku}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, c, func(s agio.State) bool { return s.Conversation(id).Model == haiku })
	if err := c.Remove(id); err != nil {
		t.Fatal(err)
	}
	waitFor(t, c, func(s agio.State) bool { return len(s.Conversations) == 0 })
	if err := c.Send("missing", agio.Message{Text: "x"}); err == nil {
		t.Fatal("sent to a missing conversation")
	}
}

func TestWrongTokenIsRefused(t *testing.T) {
	if _, _, err := connect(t, "guess"); err == nil {
		t.Fatal("connected with a wrong token")
	}
}

func TestClientReportsDisconnection(t *testing.T) {
	h, c, err := connect(t, "secret")
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, c, func(s agio.State) bool { return s.Ready() })
	h.Close()
	c.Close()
	waitFor(t, c, func(s agio.State) bool { return s.Err != "" })
}
