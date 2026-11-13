package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnvValuePlatformCase(t *testing.T) {
	env := []string{"PATH=first", "PATH=second", "Path=last", "invalid", "OTHER=a=b"}
	want := "second"
	if runtime.GOOS == "windows" {
		want = "last"
	}
	if got := EnvValue(env, "PATH"); got != want {
		t.Fatalf("PATH = %q, want %q", got, want)
	}
	if EnvValue(env, "OTHER") != "a=b" || EnvValue(env, "MISSING") != "" {
		t.Fatal("environment parsing")
	}
}

func TestLookPathWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows executable lookup")
	}
	first := filepath.Join(t.TempDir(), "first bin")
	second := filepath.Join(t.TempDir(), "second bin")
	for _, dir := range []string{first, second} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"pi", "pi.exe", "pi.com"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	// A directory named like an executable must never be returned.
	if err := os.Mkdir(filepath.Join(first, "pi.cmd"), 0755); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + second, "Path=" + first + ";" + second, "PathExt=.CMD;.EXE;.COM"}
	if got := LookPath("pi", env); got != filepath.Join(first, "pi.exe") {
		t.Fatalf("pi = %q", got)
	}
	if got := LookPath("pi.exe", env); got != filepath.Join(first, "pi.exe") {
		t.Fatalf("explicit extension = %q", got)
	}
	if got := LookPath("pi", []string{"Path=" + first}); got != filepath.Join(first, "pi.com") {
		t.Fatalf("default PATHEXT = %q", got)
	}
	if got := LookPath("missing", env); got != "" {
		t.Fatalf("missing = %q", got)
	}
	// WinGet links bun.exe into its Links directory.
	links := filepath.Join(t.TempDir(), "Links")
	if err := os.MkdirAll(links, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(second, "pi.exe")
	if err := os.Symlink(target, filepath.Join(links, "bun.exe")); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	got := LookPath("bun", []string{"Path=" + links})
	info, err := os.Lstat(got)
	want, _ := os.Stat(target)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, want) {
		t.Fatalf("linked bun = %q, want its target %q", got, target)
	}
}

func TestIsExecutablePlatformPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tool.exe")
	if err := os.WriteFile(path, []byte("fixture"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := IsExecutable(path); got != (runtime.GOOS == "windows") {
		t.Fatalf("executable without Unix permission bits = %v", got)
	}
	if IsExecutable(dir) || IsExecutable(filepath.Join(dir, "missing.exe")) {
		t.Fatal("directories and missing files are not executable")
	}
}
