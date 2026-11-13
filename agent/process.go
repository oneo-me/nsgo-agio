package agent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/oneo-me/nsgo-agio/internal/childproc"
)

// ErrClosed reports a request to a process that has exited or been closed.
var ErrClosed = errors.New("agent process is not running")

// Emitter holds events reported while a driver is still starting and
// delivers them, in order, once Open is called.
type Emitter struct {
	mu     sync.Mutex
	open   bool
	queued []Event
	fn     func(Event)
}

// NewEmitter returns a closed emitter delivering to fn.
func NewEmitter(fn func(Event)) *Emitter { return &Emitter{fn: fn} }

// Emit delivers e, or queues it until Open.
func (e *Emitter) Emit(ev Event) {
	if e == nil || e.fn == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.open {
		e.queued = append(e.queued, ev)
		return
	}
	e.fn(ev)
}

// Open delivers queued events and lets later ones through.
func (e *Emitter) Open() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, ev := range e.queued {
		e.fn(ev)
	}
	e.queued, e.open = nil, true
}

// Process is a child speaking LF-delimited JSON over stdin and stdout.
type Process struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	writeMu sync.Mutex
	done    chan struct{}
	stderr  Tail
}

// StartProcess starts cmd and calls onLine for each stdout record from one
// goroutine, then onExit once the process has exited.
func StartProcess(cmd *exec.Cmd, onLine func([]byte), onExit func(error)) (*Process, error) {
	p := &Process{cmd: cmd, done: make(chan struct{})}
	cmd.Stderr = &p.stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := childproc.Start(cmd); err != nil {
		return nil, err
	}
	p.stdin = stdin
	go func() {
		ReadLines(stdout, onLine)
		err := childproc.Wait(cmd)
		if err != nil {
			if msg := p.stderr.String(); msg != "" {
				err = fmt.Errorf("%w: %s", err, msg)
			}
		}
		close(p.done)
		if onExit != nil {
			onExit(err)
		}
	}()
	return p, nil
}

// ReadLines splits records only on LF; JSON strings may contain U+2028.
func ReadLines(r io.Reader, onLine func([]byte)) {
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := br.ReadBytes('\n')
		line = bytes.TrimSuffix(bytes.TrimSuffix(line, []byte{'\n'}), []byte{'\r'})
		if len(bytes.TrimSpace(line)) > 0 {
			onLine(line)
		}
		if err != nil {
			return
		}
	}
}

// Send writes one JSON record.
func (p *Process) Send(record any) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	if _, err := p.stdin.Write(append(data, '\n')); err != nil {
		return ErrClosed
	}
	return nil
}

// Close closes stdin and waits up to grace for the process to exit, then
// terminates its process group, including tools it left running. It never
// signals other processes.
func (p *Process) Close(grace time.Duration) {
	p.writeMu.Lock()
	p.stdin.Close()
	p.writeMu.Unlock()
	select {
	case <-p.done:
	case <-time.After(grace):
	}
	childproc.Terminate(p.cmd, time.Second)
}

// Done is closed after the process exits.
func (p *Process) Done() <-chan struct{} { return p.done }

// Stderr returns the last lines the process wrote to stderr.
func (p *Process) Stderr() string { return p.stderr.String() }

// Tail keeps the last few KiB written to it for error reports.
type Tail struct {
	mu  sync.Mutex
	buf []byte
}

func (t *Tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > 4096 {
		t.buf = t.buf[len(t.buf)-4096:]
	}
	return len(p), nil
}

// String returns the last three lines.
func (t *Tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	lines := strings.Split(strings.TrimSpace(string(t.buf)), "\n")
	if len(lines) > 3 {
		lines = lines[len(lines)-3:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
