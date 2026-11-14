package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// SkillFile names the instructions a skill directory holds.
const SkillFile = "SKILL.md"

// CheckSkill reports why dir is not a skill: it must be an absolute path
// to a directory holding SKILL.md.
func CheckSkill(dir string) error {
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("skill path is not absolute: %s", dir)
	}
	if info, err := os.Stat(filepath.Join(dir, SkillFile)); err != nil || info.IsDir() {
		return fmt.Errorf("not a skill, it has no %s: %s", SkillFile, dir)
	}
	return nil
}

// PluginDir makes a temporary plugin holding skills, in the layout Claude
// Code and CodeBuddy Code load for one session with --plugin-dir. The
// plugin is named after Client, which agents show before each skill's
// name. Skills are linked, or copied where links are refused, such as on
// Windows without Developer Mode. The caller removes the directory once
// the session ends.
func PluginDir(skills []string) (string, error) {
	dir, err := os.MkdirTemp("", Client.Name+"-skills-")
	if err != nil {
		return "", err
	}
	if err := writePlugin(dir, skills); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

func writePlugin(dir string, skills []string) error {
	manifest, _ := json.Marshal(map[string]string{"name": Client.Name})
	for _, m := range []string{".claude-plugin", ".codebuddy-plugin"} {
		if err := os.MkdirAll(filepath.Join(dir, m), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, m, "plugin.json"), manifest, 0o644); err != nil {
			return err
		}
	}
	root := filepath.Join(dir, "skills")
	if err := os.Mkdir(root, 0o755); err != nil {
		return err
	}
	for _, skill := range skills {
		name := filepath.Base(skill)
		for n := 2; ; n++ {
			if _, err := os.Lstat(filepath.Join(root, name)); errors.Is(err, os.ErrNotExist) {
				break
			}
			name = fmt.Sprintf("%s-%d", filepath.Base(skill), n)
		}
		target := filepath.Join(root, name)
		if os.Symlink(skill, target) == nil {
			continue
		}
		if err := os.CopyFS(target, os.DirFS(skill)); err != nil {
			return err
		}
	}
	return nil
}
