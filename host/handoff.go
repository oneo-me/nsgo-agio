package host

import (
	"strings"

	"github.com/oneo-me/nsgo-agio/transcript"
)

// The turns handed to an agent a conversation switched to open its first
// message between these lines, so reading its history back can drop them.
const (
	handoffOpen  = "<earlier-conversation>"
	handoffClose = "</earlier-conversation>"
)

// maxHandoff bounds the turns handed over; the oldest are dropped first.
const maxHandoff = 100_000

// handoff prefixes the first message to a new agent with the conversation
// so far: each turn's request and the agent's text, without its tool
// calls, whose results the new agent can read again in the directory.
func handoff(turns []*transcript.Turn, text string) string {
	var parts []string
	size := 0
	for i := len(turns) - 1; i >= 0; i-- {
		part := handoffTurn(turns[i])
		if part == "" {
			continue
		}
		if size += len(part); size > maxHandoff && len(parts) > 0 {
			break
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return text
	}
	var b strings.Builder
	b.WriteString(handoffOpen + "\n")
	b.WriteString("This conversation began with another coding agent in this directory and continues with you. Its turns so far follow; carry on with the same work.\n")
	for i := len(parts) - 1; i >= 0; i-- {
		b.WriteString("\n" + parts[i])
	}
	b.WriteString("\n" + handoffClose + "\n\n" + text)
	return b.String()
}

func handoffTurn(t *transcript.Turn) string {
	var b strings.Builder
	if prompt := strings.TrimSpace(t.Prompt); prompt != "" {
		b.WriteString("## User\n\n" + prompt + "\n")
		if len(t.Files) > 0 {
			b.WriteString("\nAttached files:\n- " + strings.Join(t.Files, "\n- ") + "\n")
		}
	}
	var reply []string
	for _, s := range t.Segments {
		if text := strings.TrimSpace(s.Text); s.Kind == transcript.Text && text != "" {
			reply = append(reply, text)
		}
	}
	if len(reply) > 0 {
		b.WriteString("\n## Agent\n\n" + strings.Join(reply, "\n\n") + "\n")
	}
	switch t.Status {
	case transcript.Cancelled:
		b.WriteString("\n(stopped by the user)\n")
	case transcript.Failed:
		b.WriteString("\n(failed: " + t.Err + ")\n")
	}
	return b.String()
}

// stripHandoff drops the turns handed over from a message read back.
func stripHandoff(prompt string) string {
	rest, ok := strings.CutPrefix(prompt, handoffOpen+"\n")
	if !ok {
		return prompt
	}
	if _, after, ok := strings.Cut(rest, "\n"+handoffClose+"\n\n"); ok {
		return after
	}
	return prompt
}
