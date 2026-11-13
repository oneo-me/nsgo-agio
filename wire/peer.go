package wire

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// maxFrame bounds one frame, which holds at most a state publication.
const maxFrame = 256 << 20

const (
	frameMessage byte = 1
	frameStream  byte = 2
)

// Message is a request when it has both ID and Method, its response when it
// has only ID, and an event when it has only Method.
type Message struct {
	ID     uint64          `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Handler serves a peer's requests and events, in the order they arrive.
// Request answers once through reply, at once or later from any goroutine,
// so it must not block on the work itself; Event and Stream receive what
// has no answer.
type Handler interface {
	Request(method string, params json.RawMessage, reply func(result any, err error))
	Event(method string, params json.RawMessage)
	Stream(id uint64, data []byte)
}

// ErrClosed is returned by calls on a closed peer.
var ErrClosed = errors.New("connection closed")

// Peer is one end of a connection. Its handler sees requests, events and
// streams in the order they were sent.
type Peer struct {
	rwc     io.ReadWriteCloser
	r       *bufio.Reader
	wmu     sync.Mutex
	w       *bufio.Writer
	handler Handler

	mu      sync.Mutex
	next    uint64
	pending map[uint64]chan Message
	err     error
	done    chan struct{}
}

// NewPeer starts reading rwc, delivering what arrives to h, until the
// connection closes.
func NewPeer(rwc io.ReadWriteCloser, h Handler) *Peer {
	p := &Peer{rwc: rwc, r: bufio.NewReaderSize(rwc, 64<<10), w: bufio.NewWriterSize(rwc, 64<<10), handler: h,
		pending: map[uint64]chan Message{}, done: make(chan struct{})}
	go p.read()
	return p
}

// Done closes once the connection has ended.
func (p *Peer) Done() <-chan struct{} { return p.done }

// Err is why the connection ended, once Done has closed.
func (p *Peer) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

// Close ends the connection.
func (p *Peer) Close() error {
	p.fail(ErrClosed)
	return nil
}

func (p *Peer) fail(err error) {
	p.mu.Lock()
	if p.err != nil {
		p.mu.Unlock()
		return
	}
	p.err = err
	pending := p.pending
	p.pending = nil
	p.mu.Unlock()
	p.rwc.Close()
	for _, ch := range pending {
		close(ch)
	}
	close(p.done)
}

func (p *Peer) read() {
	for {
		kind, payload, err := readFrame(p.r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = ErrClosed
			}
			p.fail(err)
			return
		}
		switch kind {
		case frameStream:
			if len(payload) < 8 {
				p.fail(fmt.Errorf("short stream frame"))
				return
			}
			p.handler.Stream(binary.BigEndian.Uint64(payload), payload[8:])
		case frameMessage:
			var m Message
			if err := json.Unmarshal(payload, &m); err != nil {
				p.fail(fmt.Errorf("decode message: %w", err))
				return
			}
			p.dispatch(m)
		}
	}
}

func (p *Peer) dispatch(m Message) {
	switch {
	case m.ID != 0 && m.Method != "":
		var once sync.Once
		p.handler.Request(m.Method, m.Params, func(result any, err error) {
			once.Do(func() {
				reply := Message{ID: m.ID}
				if err != nil {
					reply.Error = err.Error()
					if reply.Error == "" {
						reply.Error = "error"
					}
				} else if result != nil {
					data, err := json.Marshal(result)
					if err != nil {
						reply.Error = err.Error()
					} else {
						reply.Result = data
					}
				}
				p.write(reply)
			})
		})
	case m.ID != 0:
		p.mu.Lock()
		ch := p.pending[m.ID]
		delete(p.pending, m.ID)
		p.mu.Unlock()
		if ch != nil {
			ch <- m
		}
	case m.Method != "":
		p.handler.Event(m.Method, m.Params)
	}
}

// Call sends a request and waits for its response, decoding its result
// into result unless nil.
func (p *Peer) Call(ctx context.Context, method string, params, result any) error {
	ch, err := p.Send(method, params)
	if err != nil {
		return err
	}
	select {
	case m, ok := <-ch:
		return Decode(m, ok, result)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Send sends a request without waiting; its response arrives on the
// channel, which closes without one if the connection ends first.
func (p *Peer) Send(method string, params any) (<-chan Message, error) {
	data, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	ch := make(chan Message, 1)
	p.mu.Lock()
	if p.err != nil {
		err := p.err
		p.mu.Unlock()
		return nil, err
	}
	p.next++
	id := p.next
	p.pending[id] = ch
	p.mu.Unlock()
	if err := p.write(Message{ID: id, Method: method, Params: data}); err != nil {
		return nil, err
	}
	return ch, nil
}

// Decode reads a response received from a channel of Send into result.
func Decode(m Message, ok bool, result any) error {
	if !ok {
		return ErrClosed
	}
	if m.Error != "" {
		return &RemoteError{Message: m.Error}
	}
	if result == nil || len(m.Result) == 0 {
		return nil
	}
	return json.Unmarshal(m.Result, result)
}

// RemoteError is an error the other end reported.
type RemoteError struct{ Message string }

func (e *RemoteError) Error() string { return e.Message }

// Notify sends an event.
func (p *Peer) Notify(method string, params any) error {
	data, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return p.write(Message{Method: method, Params: data})
}

func (p *Peer) write(m Message) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return p.writeFrame(frameMessage, nil, data)
}

// WriteStream sends a chunk of stream id.
func (p *Peer) WriteStream(id uint64, data []byte) error {
	var head [8]byte
	binary.BigEndian.PutUint64(head[:], id)
	return p.writeFrame(frameStream, head[:], data)
}

func (p *Peer) writeFrame(kind byte, head, body []byte) error {
	p.wmu.Lock()
	defer p.wmu.Unlock()
	if err := p.Err(); err != nil {
		return err
	}
	var prefix [5]byte
	binary.BigEndian.PutUint32(prefix[:4], uint32(len(head)+len(body)))
	prefix[4] = kind
	_, err := p.w.Write(prefix[:])
	if err == nil {
		_, err = p.w.Write(head)
	}
	if err == nil {
		_, err = p.w.Write(body)
	}
	if err == nil {
		err = p.w.Flush()
	}
	if err != nil {
		go p.fail(err)
	}
	return err
}

func readFrame(r *bufio.Reader) (byte, []byte, error) {
	var prefix [5]byte
	if _, err := io.ReadFull(r, prefix[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(prefix[:4])
	if n > maxFrame {
		return 0, nil, fmt.Errorf("frame of %d bytes exceeds the limit", n)
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return 0, nil, err
	}
	return prefix[4], payload, nil
}
