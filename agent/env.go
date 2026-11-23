package agent

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
)

// InheritedPath stands for the agent's own PATH in a session's PATH, which
// may only add directories before or after it.
const InheritedPath = "$PATH"

// isPath reports whether key names PATH, case-insensitively on Windows.
func isPath(key string) bool {
	return key == "PATH" || runtime.GOOS == "windows" && strings.EqualFold(key, "PATH")
}

// CheckEnv reports why env cannot be a session's environment: names must be
// non-empty without '=' or NUL, values without NUL, and PATH must keep the
// agent's own by holding InheritedPath once.
func CheckEnv(env map[string]string) error {
	for key, value := range env {
		if key == "" || strings.ContainsAny(key, "=\x00") {
			return fmt.Errorf("invalid environment variable name %q", key)
		}
		if strings.ContainsRune(value, 0) {
			return fmt.Errorf("environment variable %s holds NUL", key)
		}
		if isPath(key) && strings.Count(value, InheritedPath) != 1 {
			return errors.New("PATH must hold " + InheritedPath + " once, to add directories before or after the agent's own")
		}
	}
	return nil
}

// MergeEnv returns base with env set over it, its PATH holding base's PATH
// in place of InheritedPath. base is not modified.
func MergeEnv(base []string, env map[string]string) []string {
	if len(env) == 0 {
		return base
	}
	merged := slices.Clone(base)
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		value := env[key]
		if isPath(key) {
			value = joinPath(value, EnvValue(base, key))
		}
		merged = append(merged, key+"="+value)
	}
	return merged
}

// joinPath replaces InheritedPath in path with inherited, dropping the list
// separator beside it when inherited is empty.
func joinPath(path, inherited string) string {
	if inherited == "" {
		sep := string(os.PathListSeparator)
		for _, s := range []string{InheritedPath + sep, sep + InheritedPath} {
			if strings.Contains(path, s) {
				return strings.Replace(path, s, "", 1)
			}
		}
	}
	return strings.Replace(path, InheritedPath, inherited, 1)
}
