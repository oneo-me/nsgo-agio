package wire

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// recorder serves requests by method and records what else arrives.
type recorder struct {
	mu     sync.Mutex
	events []string
	stream []byte
	held   func(any, error)
}

func (r *recorder) Request(method string, params json.RawMessage, reply func(any, error)) {
	switch method {
	case "echo":
		var s string
		json.Unmarshal(params, &s)
		reply(s+"!", nil)
	case "fail":
		reply(nil, errors.New("no such thing"))
	case "later":
		// Answered from another goroutine, twice; only the first counts.
		go func() {
			reply("first", nil)
			reply("second", nil)
		}()
	case "hold":
		r.mu.Lock()
		r.held = reply
		r.mu.Unlock()
	}
}

func (r *recorder) Event(method string, params json.RawMessage) {
	r.mu.Lock()
	r.events = append(r.events, method+string(params))
	r.mu.Unlock()
}

func (r *recorder) Stream(id uint64, data []byte) {
	r.mu.Lock()
	r.stream = append(r.stream, data...)
	r.mu.Unlock()
}

func pair(t *testing.T) (*Peer, *Peer, *recorder) {
	t.Helper()
	a, b := net.Pipe()
	server := &recorder{}
	client, srv := NewPeer(a, &recorder{}), NewPeer(b, server)
	t.Cleanup(func() { client.Close(); srv.Close() })
	return client, srv, server
}

func TestPeersExchangeRequestsEventsAndStreams(t *testing.T) {
	client, _, server := pair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var got string
	if err := client.Call(ctx, "echo", "hi", &got); err != nil || got != "hi!" {
		t.Fatalf("echo: %q %v", got, err)
	}
	var remote *RemoteError
	if err := client.Call(ctx, "fail", nil, nil); !errors.As(err, &remote) || remote.Message != "no such thing" {
		t.Fatalf("remote error: %v", err)
	}
	if err := client.Call(ctx, "later", nil, &got); err != nil || got != "first" {
		t.Fatalf("deferred reply: %q %v", got, err)
	}
	// Events and streams arrive in the order sent, before a later reply.
	client.Notify("one", 1)
	client.WriteStream(7, []byte("out"))
	client.WriteStream(7, []byte("put"))
	client.Notify("two", 2)
	if err := client.Call(ctx, "echo", "", nil); err != nil {
		t.Fatal(err)
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if strings.Join(server.events, ",") != "one1,two2" || string(server.stream) != "output" {
		t.Fatalf("events %v, stream %q", server.events, server.stream)
	}
}

func TestClosingEndsPendingCalls(t *testing.T) {
	client, srv, _ := pair(t)
	ch, err := client.Send("hold", nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.Close()
	select {
	case m, ok := <-ch:
		if err := Decode(m, ok, nil); !errors.Is(err, ErrClosed) {
			t.Fatalf("pending call: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pending call never ended")
	}
	select {
	case <-client.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the other end did not notice")
	}
	if _, err := client.Send("echo", ""); err == nil {
		t.Fatal("sent on a closed connection")
	}
}

func TestOversizedFrameEndsTheConnection(t *testing.T) {
	a, b := net.Pipe()
	p := NewPeer(b, &recorder{})
	defer p.Close()
	var prefix [5]byte
	binary.BigEndian.PutUint32(prefix[:4], maxFrame+1)
	prefix[4] = frameMessage
	go a.Write(prefix[:])
	select {
	case <-p.Done():
		if err := p.Err(); err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Fatalf("error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("oversized frame accepted")
	}
}
