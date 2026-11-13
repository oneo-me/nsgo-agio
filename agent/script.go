package agent

import (
	"bufio"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode/utf16"
)

// LocateScript finds the executable name on env's PATH, then in Bun's
// global bin, and returns how to start it: a program and its leading
// arguments. Node scripts installed by Bun, or whose node shebang cannot
// resolve, run as `bun run <script>`. On Windows that is the script Bun's
// name.exe shim would start, bypassing the shim, which can fail to start it.
// Native executables run directly. The path is "" when name is not found.
func LocateScript(name string, env []string) (string, []string) {
	bin := bunBin(env)
	path := LookPath(name, env)
	if path == "" && bin != "" {
		path = lookInDir(name, bin, env)
	}
	if path == "" {
		return "", nil
	}
	script, resolved := path, ""
	shim := bunShimScript(path)
	if shim != "" {
		script, resolved = shim, shim
	} else if r, err := filepath.EvalSymlinks(path); err == nil {
		resolved = r
	}
	if !usesNode(resolved) {
		return path, nil
	}
	installedByBun := shim != "" || bunInstalled(path, resolved) || bin != "" && filepath.Dir(path) == bin
	if installedByBun || LookPath("node", env) == "" {
		bun := LookPath("bun", env)
		if bun == "" && installedByBun {
			// Bun's own bin directory may be absent from a minimal PATH.
			bun = lookInDir("bun", filepath.Dir(path), env)
		}
		if bun != "" {
			return bun, []string{"run", script}
		}
	}
	return path, nil
}

// bunShimScript returns the script that Bun's Windows shim at path starts,
// or "" when path is not one. Bun installs a script's bin as name.exe beside
// name.bunx, which starts with the script's path in UTF-16LE, ended by `"`
// and NUL, relative to the parent of the shim's directory unless absolute.
func bunShimScript(path string) string {
	ext := filepath.Ext(path)
	if !strings.EqualFold(ext, ".exe") {
		return ""
	}
	data, err := os.ReadFile(strings.TrimSuffix(path, ext) + ".bunx")
	if err != nil {
		return ""
	}
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(data[2*i:])
	}
	end := slices.Index(units, '"')
	if end <= 0 || end+1 == len(units) || units[end+1] != 0 {
		return ""
	}
	script := filepath.FromSlash(strings.ReplaceAll(string(utf16.Decode(units[:end])), `\`, "/"))
	if !filepath.IsAbs(script) {
		script = filepath.Join(filepath.Dir(filepath.Dir(path)), script)
	}
	return script
}

func bunInstalled(paths ...string) bool {
	for _, p := range paths {
		if strings.Contains(filepath.ToSlash(p), "/.bun/") {
			return true
		}
	}
	return false
}

func bunBin(env []string) string {
	if root := EnvValue(env, "BUN_INSTALL"); root != "" {
		return filepath.Join(root, "bin")
	}
	home := EnvValue(env, "HOME")
	if runtime.GOOS == "windows" {
		if profile := EnvValue(env, "USERPROFILE"); profile != "" {
			home = profile
		}
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".bun", "bin")
}

func lookInDir(name, dir string, env []string) string {
	return LookPath(name, []string{"PATH=" + dir, "PATHEXT=" + EnvValue(env, "PATHEXT")})
}

func usesNode(script string) bool {
	f, err := os.Open(script)
	if err != nil {
		return false
	}
	defer f.Close()
	line, _ := bufio.NewReader(f).ReadString('\n')
	return strings.HasPrefix(line, "#!") && strings.Contains(line, "node")
}
