package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocateScriptWindowsBun(t *testing.T) {
	for _, scenario := range []string{"Path", "USERPROFILE", "HOME", "BUN_INSTALL", "PATH wins", "missing", "Bun shim"} {
		t.Run(scenario, func(t *testing.T) {
			home := t.TempDir()
			bin := filepath.Join(home, ".bun", "bin")
			if scenario == "BUN_INSTALL" {
				bin = filepath.Join(home, "custom bun", "bin")
			}
			if err := os.MkdirAll(bin, 0755); err != nil {
				t.Fatal(err)
			}
			shim := filepath.Join(bin, "pi.exe")
			// A native pi.exe has no .bunx beside it, node shebang or Unix mode bits.
			if err := os.WriteFile(shim, []byte("MZ fixture"), 0644); err != nil {
				t.Fatal(err)
			}
			var env []string
			switch scenario {
			case "Path":
				env = []string{"Path=" + bin, "PATHEXT=.COM;.EXE;.BAT;.CMD"}
			case "USERPROFILE":
				env = []string{"UserProfile=" + home, "HOME=" + t.TempDir()}
			case "HOME":
				env = []string{"HOME=" + home}
			case "BUN_INSTALL":
				env = []string{"Bun_Install=" + filepath.Dir(bin)}
			case "PATH wins":
				other := filepath.Join(t.TempDir(), "pi.exe")
				if err := os.WriteFile(other, []byte("MZ other"), 0644); err != nil {
					t.Fatal(err)
				}
				env = []string{"Path=" + filepath.Dir(other), "USERPROFILE=" + home}
				shim = other
			case "missing":
				env = []string{"Path=" + t.TempDir(), "USERPROFILE=" + t.TempDir()}
			case "Bun shim":
				// WinGet puts bun.exe on Path, outside ~/.bun/bin, which is not.
				script := filepath.Join(home, ".bun", "install", "global", "node_modules", "pi", "cli.js")
				if err := os.MkdirAll(filepath.Dir(script), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(script, []byte("#!/usr/bin/env node\n"), 0644); err != nil {
					t.Fatal(err)
				}
				writeBunShim(t, shim, `install\global\node_modules\pi\cli.js`)
				links := t.TempDir()
				bun := filepath.Join(links, "bun.exe")
				if err := os.WriteFile(bun, []byte("MZ bun"), 0644); err != nil {
					t.Fatal(err)
				}
				env = []string{"Path=" + links, "USERPROFILE=" + home}
				path, args := LocateScript("pi", env)
				if path != bun || strings.Join(args, "|") != "run|"+script {
					t.Fatalf("Bun shim: %s %v; want %s run %s", path, args, bun, script)
				}
				return
			}
			path, args := LocateScript("pi", env)
			if scenario == "missing" {
				if path != "" {
					t.Fatalf("missing: %s %v", path, args)
				}
				return
			}
			if path != shim || len(args) != 0 {
				t.Fatalf("native shim: %s %v; want %s without bun run", path, args, shim)
			}
		})
	}
}
