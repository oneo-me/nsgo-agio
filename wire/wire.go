// Package wire carries an agio.Backend between processes: Serve serves one
// to a client over any byte stream, such as a TCP connection or an SSH
// session's standard input and output, and a Client mirrors it as an
// agio.Backend of its own. Frames hold JSON requests, responses and events;
// conversations travel as transcript patches, so a streamed reply sends
// only what arrived.
package wire

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/transcript"
)

// Version names the protocol. A client and server of different versions
// only exchange Hello.
const Version = 1

// Requests a client sends, after Hello; the server publishes EventState.
const (
	MethodHello    = "hello"
	MethodCreate   = "create"
	MethodSend     = "send"
	MethodStop     = "stop"
	MethodChoose   = "choose"
	MethodRemove   = "remove"
	MethodDiscover = "discover"
	EventState     = "state"
)

// Hello opens a session. Token must match the server's when it has one.
type Hello struct {
	Version int    `json:"version"`
	Token   string `json:"token,omitempty"`
}

// Welcome answers Hello.
type Welcome struct {
	Version int `json:"version"`
}

// Ref names a conversation.
type Ref struct {
	ID string `json:"id"`
}

// Send sends a message to a conversation.
type Send struct {
	ID      string       `json:"id"`
	Message agio.Message `json:"message"`
}

// Choose changes a conversation's choices.
type Choose struct {
	ID     string      `json:"id"`
	Choice agio.Choice `json:"choice"`
}

// State is what changed since the last publication. Reset replaces the
// client's copy; otherwise nil Head and Conversations are unchanged and
// Turns patch only the conversations they name.
type State struct {
	Reset         bool                        `json:"reset,omitempty"`
	Head          *Head                       `json:"head,omitempty"`
	Conversations *[]*agio.Conversation       `json:"conversations,omitempty"`
	Turns         map[string]transcript.Patch `json:"turns,omitempty"`
}

// Head is the backend's state but its conversations.
type Head struct {
	Discovering bool        `json:"discovering,omitempty"`
	Tools       []agio.Tool `json:"tools,omitempty"`
}

// publishInterval bounds how often a client is sent changes.
const publishInterval = 33 * time.Millisecond

// callTimeout bounds a request; backends answer at once.
const callTimeout = 15 * time.Second

// Serve serves b to the client on conn until either ends. A nonempty token
// must match the client's.
func Serve(b agio.Backend, conn io.ReadWriteCloser, token string) error {
	s := &session{b: b, token: token, kick: make(chan struct{}, 1), closed: make(chan struct{})}
	s.peer = NewPeer(conn, s)
	stop := b.Watch(s.changed)
	defer stop()
	go s.write()
	<-s.peer.Done()
	close(s.closed)
	return s.peer.Err()
}

// ListenAndServe serves b to every client connecting to addr over TCP. Keep
// addr on a loopback or private network, or set a token: a client controls
// agents with full access to the server's files.
func ListenAndServe(b agio.Backend, addr, token string) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer l.Close()
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		go Serve(b, conn, token)
	}
}

// session is one client's connection.
type session struct {
	b     agio.Backend
	token string
	peer  *Peer

	mu       sync.Mutex
	accepted bool
	kick     chan struct{}
	closed   chan struct{}

	// Writer-owned: what the client was sent.
	sentHead          []byte
	sentConversations []byte
	sentTurns         map[string][]*transcript.Turn
}

func (s *session) changed() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// write publishes what changed at most once per publishInterval.
func (s *session) write() {
	for {
		select {
		case <-s.kick:
		case <-s.closed:
			return
		}
		s.mu.Lock()
		accepted := s.accepted
		s.mu.Unlock()
		if accepted {
			if out := s.delta(); out != nil {
				if err := s.peer.Notify(EventState, out); err != nil {
					return
				}
			}
		}
		select {
		case <-time.After(publishInterval):
		case <-s.closed:
			return
		}
	}
}

// delta is what changed since the last publication, or nil.
func (s *session) delta() *State {
	st := s.b.State()
	out := &State{}
	changed := false
	if s.sentTurns == nil {
		out.Reset, changed, s.sentTurns = true, true, map[string][]*transcript.Turn{}
	}
	head := Head{Discovering: st.Discovering, Tools: st.Tools}
	if data, _ := json.Marshal(head); out.Reset || string(data) != string(s.sentHead) {
		s.sentHead, out.Head, changed = data, &head, true
	}
	if data, _ := json.Marshal(st.Conversations); out.Reset || string(data) != string(s.sentConversations) {
		s.sentConversations, changed = data, true
		// An empty list, not null, says the last one went.
		list := append([]*agio.Conversation{}, st.Conversations...)
		out.Conversations = &list
	}
	seen := map[string]bool{}
	for _, c := range st.Conversations {
		seen[c.ID] = true
		if p, ok := transcript.Diff(s.sentTurns[c.ID], c.Turns); ok {
			if out.Turns == nil {
				out.Turns = map[string]transcript.Patch{}
			}
			out.Turns[c.ID] = p
			changed = true
		}
		s.sentTurns[c.ID] = c.Turns
	}
	for id := range s.sentTurns {
		if !seen[id] {
			delete(s.sentTurns, id)
		}
	}
	if !changed {
		return nil
	}
	return out
}

var errHello = errors.New("hello first")

// Request implements Handler.
func (s *session) Request(method string, params json.RawMessage, reply func(any, error)) {
	decode := func(v any) error { return json.Unmarshal(params, v) }
	if method == MethodHello {
		var h Hello
		if err := decode(&h); err != nil {
			reply(nil, err)
			return
		}
		if h.Version != Version {
			reply(Welcome{Version: Version}, nil)
			return
		}
		if s.token != "" && subtle.ConstantTimeCompare([]byte(h.Token), []byte(s.token)) != 1 {
			reply(nil, errors.New("wrong token"))
			return
		}
		s.mu.Lock()
		s.accepted = true
		s.mu.Unlock()
		reply(Welcome{Version: Version}, nil)
		s.changed()
		return
	}
	s.mu.Lock()
	accepted := s.accepted
	s.mu.Unlock()
	if !accepted {
		reply(nil, errHello)
		return
	}
	switch method {
	case MethodCreate:
		var m agio.Create
		if err := decode(&m); err != nil {
			reply(nil, err)
			return
		}
		id, err := s.b.Create(m)
		if err != nil {
			reply(nil, err)
			return
		}
		reply(Ref{ID: id}, nil)
	case MethodSend:
		var m Send
		if err := decode(&m); err != nil {
			reply(nil, err)
			return
		}
		reply(nil, s.b.Send(m.ID, m.Message))
	case MethodChoose:
		var m Choose
		if err := decode(&m); err != nil {
			reply(nil, err)
			return
		}
		reply(nil, s.b.Choose(m.ID, m.Choice))
	case MethodStop, MethodRemove:
		var m Ref
		if err := decode(&m); err != nil {
			reply(nil, err)
			return
		}
		if method == MethodStop {
			reply(nil, s.b.Stop(m.ID))
		} else {
			reply(nil, s.b.Remove(m.ID))
		}
	case MethodDiscover:
		s.b.Discover()
		reply(nil, nil)
	default:
		reply(nil, errors.New("unknown request "+method))
	}
}

// Event implements Handler; clients send none.
func (s *session) Event(string, json.RawMessage) {}

// Stream implements Handler; clients send none.
func (s *session) Stream(uint64, []byte) {}
