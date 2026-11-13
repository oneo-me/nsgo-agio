package agent

import "testing"

func TestKeysKeepPiCompatible(t *testing.T) {
	for _, c := range []struct {
		key  string
		tool Tool
		id   string
	}{
		{"anthropic/claude-opus", Pi, "anthropic/claude-opus"},
		{"codex:gpt-6-luna", Codex, "gpt-6-luna"},
		{"claude:opus", Claude, "opus"},
		{"opencode:ollama/qwen3:8b", "opencode", "ollama/qwen3:8b"},
		{"pi:p/m", Pi, "p/m"},
		{"custom:x", Pi, "custom:x"},
	} {
		tool, id := ParseKey(c.key)
		if tool != c.tool || id != c.id {
			t.Errorf("%s: %s %s", c.key, tool, id)
		}
	}
	if Key(Pi, "p/m") != "p/m" || Key(Codex, "m") != "codex:m" || Tool("opencode").Name() != "OpenCode" {
		t.Fatal("keys")
	}
	if FirstLine(" a\nb ") != "a …" || Preview([]byte(`{"command":"ls\n-la"}`)) != "ls …" {
		t.Fatal("previews")
	}
}

func TestThinkingOff(t *testing.T) {
	for level, want := range map[string]bool{"off": true, "none": true, "minimal": false, "high": false, "": false} {
		if got := ThinkingOff(level); got != want {
			t.Errorf("ThinkingOff(%q) = %v, want %v", level, got, want)
		}
	}
}

func TestEmitterQueuesUntilOpen(t *testing.T) {
	var got []EventType
	e := NewEmitter(func(ev Event) { got = append(got, ev.Type) })
	e.Emit(Event{Type: Started})
	if len(got) != 0 {
		t.Fatal("delivered before Open")
	}
	e.Open()
	e.Emit(Event{Type: Settled})
	if len(got) != 2 || got[0] != Started || got[1] != Settled {
		t.Fatalf("order: %v", got)
	}
}
