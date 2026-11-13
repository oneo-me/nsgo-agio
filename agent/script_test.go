package agent

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf16"
)

func writeBunShim(t *testing.T, exe, target string) {
	t.Helper()
	if err := os.WriteFile(exe, []byte("MZ shim"), 0644); err != nil {
		t.Fatal(err)
	}
	units := utf16.Encode([]rune(target + "\"\x00node "))
	var data []byte
	for _, u := range units {
		data = binary.LittleEndian.AppendUint16(data, u)
	}
	data = binary.LittleEndian.AppendUint32(data, uint32(2*len(utf16.Encode([]rune(target)))))
	data = binary.LittleEndian.AppendUint32(data, uint32(2*len("node ")))
	data = binary.LittleEndian.AppendUint16(data, 0b111|2739<<4) // node shebang, version 5
	if err := os.WriteFile(strings.TrimSuffix(exe, ".exe")+".bunx", data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestBunShimScript(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(bin, "pi.exe")
	writeBunShim(t, shim, `install\global\node_modules\@earendil-works\pi-coding-agent\dist\bundle\cli.js`)
	want := filepath.Join(root, "install", "global", "node_modules", "@earendil-works", "pi-coding-agent", "dist", "bundle", "cli.js")
	if got := bunShimScript(shim); got != want {
		t.Fatalf("relative target: %q, want %q", got, want)
	}
	absolute := filepath.Join(t.TempDir(), "cli.js")
	writeBunShim(t, shim, absolute)
	if got := bunShimScript(shim); got != absolute {
		t.Fatalf("absolute target: %q, want %q", got, absolute)
	}
	if err := os.WriteFile(filepath.Join(bin, "pi.bunx"), []byte("corrupt"), 0644); err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(bin, "native.exe")
	if err := os.WriteFile(native, []byte("MZ native"), 0644); err != nil {
		t.Fatal(err)
	}
	if bunShimScript(shim) != "" || bunShimScript(native) != "" || bunShimScript(filepath.Join(bin, "pi")) != "" {
		t.Fatal("not a Bun shim")
	}
}

func TestLocateScriptPrefersBunForNodeScriptWithoutNode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix script and symlink fixture; Windows uses native shims")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "lib", "cli.js")
	os.MkdirAll(filepath.Dir(script), 0755)
	os.WriteFile(script, []byte("#!/usr/bin/env node\n"), 0755)
	bin := filepath.Join(dir, "bin")
	os.MkdirAll(bin, 0755)
	os.Symlink(script, filepath.Join(bin, "pi"))
	env := []string{"PATH=" + bin}
	path, args := LocateScript("pi", env)
	if path != filepath.Join(bin, "pi") || args != nil {
		t.Fatalf("without bun: %s %v", path, args)
	}
	os.WriteFile(filepath.Join(bin, "bun"), []byte("#!/bin/sh\n"), 0755)
	path, args = LocateScript("pi", env)
	if path != filepath.Join(bin, "bun") || strings.Join(args, " ") != "run "+filepath.Join(bin, "pi") {
		t.Fatalf("with bun: %s %v", path, args)
	}
	if path, _ := LocateScript("pi", []string{"PATH=" + t.TempDir()}); path != "" {
		t.Fatal("missing pi")
	}

	// A Bun global install runs under Bun even when node exists, and is found
	// in ~/.bun/bin when that directory is missing from PATH.
	home := t.TempDir()
	bunBin := filepath.Join(home, ".bun", "bin")
	os.MkdirAll(bunBin, 0755)
	os.Symlink(script, filepath.Join(bunBin, "pi"))
	os.WriteFile(filepath.Join(bunBin, "bun"), []byte("#!/bin/sh\n"), 0755)
	nodeDir := t.TempDir()
	os.WriteFile(filepath.Join(nodeDir, "node"), []byte("#!/bin/sh\n"), 0755)
	path, args = LocateScript("pi", []string{"HOME=" + home, "PATH=" + nodeDir})
	if path != filepath.Join(bunBin, "bun") || strings.Join(args, " ") != "run "+filepath.Join(bunBin, "pi") {
		t.Fatalf("bun install: %s %v", path, args)
	}

	// A custom Bun root is also recognized, even with node on PATH.
	root := filepath.Join(t.TempDir(), "custom bun")
	bin = filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(script, filepath.Join(bin, "pi")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "bun"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	path, args = LocateScript("pi", []string{"BUN_INSTALL=" + root, "HOME=" + home, "PATH=" + nodeDir})
	if path != filepath.Join(bin, "bun") || strings.Join(args, " ") != "run "+filepath.Join(bin, "pi") {
		t.Fatalf("custom bun install: %s %v", path, args)
	}
}
