package acpcli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/oneo-me/nsgo-agio/agent"
)

func TestEveryOtherToolServesACP(t *testing.T) {
	for _, tool := range agent.Tools {
		switch tool {
		case agent.Pi, agent.Codex, agent.Claude, agent.CodeBuddy:
			continue
		}
		if cli, ok := CLIs[tool]; !ok || cli.Spec.Tool != tool || len(cli.Names) == 0 {
			t.Fatalf("%s: %+v", tool, cli)
		}
	}
}

func TestLocateTriesEachNameAndReadsTheEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake CLIs are shell scripts")
	}
	bin := t.TempDir()
	for _, name := range []string{"agent", "grok"} {
		os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"), 0o755)
	}
	env := []string{"PATH=" + bin}
	d, err := Locate(agent.Cursor, env)
	if err != nil || d.Path != filepath.Join(bin, "agent") || d.AuthMethod != "cursor_login" {
		t.Fatalf("cursor: %+v, %v", d, err)
	}
	if d, _ := Locate(agent.Grok, env); d.AuthMethod != "cached_token" {
		t.Fatalf("grok: %+v", d.Spec)
	}
	if d, _ := Locate(agent.Grok, append(env, "XAI_API_KEY=k")); d.AuthMethod != "xai.api_key" || CLIs[agent.Grok].Spec.AuthMethod != "cached_token" {
		t.Fatalf("grok with a key: %+v", d.Spec)
	}
	if _, err := Locate(agent.Kiro, env); err == nil {
		t.Fatal("found kiro-cli")
	}
}
