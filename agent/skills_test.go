package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckSkill(t *testing.T) {
	skill := t.TempDir()
	if CheckSkill(skill) == nil || CheckSkill("relative") == nil {
		t.Fatal("accepted a directory without SKILL.md or a relative path")
	}
	os.WriteFile(filepath.Join(skill, SkillFile), []byte("---\nname: x\n---\n"), 0o644)
	if err := CheckSkill(skill); err != nil {
		t.Fatal(err)
	}
}

func TestPluginDirLinksSkillsUnderTheClientsName(t *testing.T) {
	a, b := filepath.Join(t.TempDir(), "review"), filepath.Join(t.TempDir(), "review")
	for _, dir := range []string{a, b} {
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, SkillFile), []byte(dir), 0o644)
	}
	dir, err := PluginDir([]string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	for _, m := range []string{".claude-plugin", ".codebuddy-plugin"} {
		var manifest struct{ Name string }
		data, _ := os.ReadFile(filepath.Join(dir, m, "plugin.json"))
		if json.Unmarshal(data, &manifest); manifest.Name != Client.Name {
			t.Fatalf("%s: %s", m, data)
		}
	}
	for name, want := range map[string]string{"review": a, "review-2": b} {
		if data, _ := os.ReadFile(filepath.Join(dir, "skills", name, SkillFile)); string(data) != want {
			t.Fatalf("%s holds %q, want %q", name, data, want)
		}
	}
}
