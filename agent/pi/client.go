package pi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/internal/childproc"
)

// RequestTimeout bounds a command response. Prompt responses arrive on
// acceptance, not completion, so this also suits prompts.
const RequestTimeout = 30 * time.Second

// ErrClosed reports a request to a process that has exited or been closed.
var ErrClosed = errors.New("pi RPC process is not running")

// Options starts an RPC process in Dir. Session resumes a session file.
type Options struct {
	Command Command
	Dir     string
	Session string
	// Args are extra CLI arguments, such as --no-session.
	Args []string
}

// Client is one pi RPC process. Events and exit are reported from a single
// reader goroutine in stdout order; callers move them to their UI thread.
type Client struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[string]chan response
	next    uint64
	closed  bool
	done    chan struct{}
	stderr  tail
}

type response struct {
	data json.RawMessage
	err  error
}

// Start launches `pi --mode rpc --approve`. Agio always runs pi with full
// access in a dedicated worktree, so project-local resources are trusted.
func Start(o Options, onEvent func(Event), onExit func(error)) (*Client, error) {
	args := []string{"--mode", "rpc", "--approve"}
	if o.Session != "" {
		args = append(args, "--session", o.Session)
	}
	cmd := o.Command.Exec(context.Background(), append(args, o.Args...)...)
	cmd.Dir = o.Dir
	c := &Client{cmd: cmd, pending: map[string]chan response{}, done: make(chan struct{})}
	cmd.Stderr = &c.stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := childproc.Start(cmd); err != nil {
		return nil, fmt.Errorf("start pi: %w", err)
	}
	c.stdin = stdin
	go func() {
		c.read(stdout, onEvent)
		err := childproc.Wait(cmd)
		if err != nil {
			if msg := c.stderr.String(); msg != "" {
				err = fmt.Errorf("%w: %s", err, msg)
			}
		}
		c.fail(ErrClosed)
		close(c.done)
		if onExit != nil {
			onExit(err)
		}
	}()
	return c, nil
}

// read splits records only on LF; JSON strings may contain U+2028/U+2029.
func (c *Client) read(r io.Reader, onEvent func(Event)) {
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := br.ReadBytes('\n')
		line = bytes.TrimSuffix(bytes.TrimSuffix(line, []byte{'\n'}), []byte{'\r'})
		if len(bytes.TrimSpace(line)) > 0 {
			c.dispatch(line, onEvent)
		}
		if err != nil {
			return
		}
	}
}

func (c *Client) dispatch(line []byte, onEvent func(Event)) {
	var head struct {
		Type    string          `json:"type"`
		ID      string          `json:"id"`
		Success bool            `json:"success"`
		Error   string          `json:"error"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(line, &head); err != nil {
		return
	}
	if head.Type == "response" {
		c.mu.Lock()
		ch := c.pending[head.ID]
		delete(c.pending, head.ID)
		c.mu.Unlock()
		if ch != nil {
			if head.Success {
				ch <- response{data: head.Data}
			} else {
				ch <- response{err: errors.New(firstNonEmpty(head.Error, "pi rejected the command"))}
			}
		}
		return
	}
	var ev Event
	if err := json.Unmarshal(line, &ev); err == nil && onEvent != nil {
		onEvent(ev)
	}
}

// Call sends a command and waits for its response data.
func (c *Client) Call(ctx context.Context, command map[string]any) (json.RawMessage, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrClosed
	}
	c.next++
	id := agent.Client.Name + "-" + strconv.FormatUint(c.next, 10)
	ch := make(chan response, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	record := map[string]any{"id": id}
	for k, v := range command {
		record[k] = v
	}
	if err := c.Send(record); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()
	select {
	case r := <-ch:
		return r.data, r.err
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("%v: %w", command["type"], ctx.Err())
	}
}

// CallInto decodes a command's data into out.
func (c *Client) CallInto(ctx context.Context, command map[string]any, out any) error {
	data, err := c.Call(ctx, command)
	if err != nil || out == nil || len(data) == 0 {
		return err
	}
	return json.Unmarshal(data, out)
}

// Send writes one record without waiting, as for extension UI responses.
func (c *Client) Send(record map[string]any) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		return ErrClosed
	}
	return nil
}

func (c *Client) fail(err error) {
	c.mu.Lock()
	c.closed = true
	pending := c.pending
	c.pending = map[string]chan response{}
	c.mu.Unlock()
	for _, ch := range pending {
		ch <- response{err: err}
	}
}

// Close asks pi to shut down by closing stdin and waits up to grace, then
// terminates its process group, including tools it left running and the
// Node process under `bun run`. It never signals other processes.
func (c *Client) Close(grace time.Duration) {
	c.writeMu.Lock()
	c.stdin.Close()
	c.writeMu.Unlock()
	select {
	case <-c.done:
	case <-time.After(grace):
	}
	childproc.Terminate(c.cmd, time.Second)
}

// Done is closed after the process exits.
func (c *Client) Done() <-chan struct{} { return c.done }

// tail keeps the last few KiB of stderr for error reports.
type tail struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > 4096 {
		t.buf = t.buf[len(t.buf)-4096:]
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	lines := strings.Split(strings.TrimSpace(string(t.buf)), "\n")
	if len(lines) > 3 {
		lines = lines[len(lines)-3:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
