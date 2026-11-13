// Package launchers finds the file manager, terminals, development tools and
// Git clients installed on this computer and opens directories with them.
package launchers

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/oneo-me/nsgo-agio/internal/childproc"
)

// Kinds of launcher, and the choice hiding a kind's button; an empty choice
// picks the first installed launcher of its kind.
const (
	Files    = "files"
	Terminal = "terminal"
	Editor   = "editor"
	Git      = "git"
	None     = "none"
)

// Launcher is a tool that opens a directory.
type Launcher struct {
	ID, Name, Kind string
	// App is the macOS application bundle name.
	App string
	// Command runs the launcher on other platforms with Args(dir).
	Command string
	Args    func(dir string) []string
}

func dirArg(dir string) []string { return []string{dir} }

func flagArg(flag string) func(string) []string {
	return func(dir string) []string { return []string{flag + dir} }
}

// Catalog lists common tools in their automatic preference order.
var Catalog = []Launcher{
	{ID: "finder", Name: "Finder", Kind: Files, App: "Finder"},
	{ID: "explorer", Name: "File Explorer", Kind: Files, Command: "explorer", Args: dirArg},
	{ID: "xdg-open", Name: "Files", Kind: Files, Command: "xdg-open", Args: dirArg},

	{ID: "apple-terminal", Name: "Terminal", Kind: Terminal, App: "Terminal"},
	{ID: "ghostty", Name: "Ghostty", Kind: Terminal, App: "Ghostty", Command: "ghostty", Args: flagArg("--working-directory=")},
	{ID: "iterm", Name: "iTerm", Kind: Terminal, App: "iTerm"},
	{ID: "warp", Name: "Warp", Kind: Terminal, App: "Warp"},
	{ID: "windows-terminal", Name: "Windows Terminal", Kind: Terminal, Command: "wt", Args: func(dir string) []string { return []string{"-d", dir} }},
	{ID: "gnome-terminal", Name: "GNOME Terminal", Kind: Terminal, Command: "gnome-terminal", Args: flagArg("--working-directory=")},
	{ID: "konsole", Name: "Konsole", Kind: Terminal, Command: "konsole", Args: func(dir string) []string { return []string{"--workdir", dir} }},
	{ID: "kitty", Name: "kitty", Kind: Terminal, Command: "kitty", Args: flagArg("--directory=")},
	{ID: "wezterm", Name: "WezTerm", Kind: Terminal, Command: "wezterm", Args: func(dir string) []string { return []string{"start", "--cwd", dir} }},
	{ID: "alacritty", Name: "Alacritty", Kind: Terminal, Command: "alacritty", Args: func(dir string) []string { return []string{"--working-directory", dir} }},

	{ID: "vscode", Name: "Visual Studio Code", Kind: Editor, App: "Visual Studio Code", Command: "code", Args: dirArg},
	{ID: "cursor", Name: "Cursor", Kind: Editor, App: "Cursor", Command: "cursor", Args: dirArg},
	{ID: "windsurf", Name: "Windsurf", Kind: Editor, App: "Windsurf", Command: "windsurf", Args: dirArg},
	{ID: "zed", Name: "Zed", Kind: Editor, App: "Zed", Command: "zed", Args: dirArg},
	{ID: "sublime-text", Name: "Sublime Text", Kind: Editor, App: "Sublime Text", Command: "subl", Args: dirArg},
	{ID: "nova", Name: "Nova", Kind: Editor, App: "Nova"},
	{ID: "xcode", Name: "Xcode", Kind: Editor, App: "Xcode"},
	{ID: "intellij-idea", Name: "IntelliJ IDEA", Kind: Editor, App: "IntelliJ IDEA", Command: "idea", Args: dirArg},
	{ID: "goland", Name: "GoLand", Kind: Editor, App: "GoLand", Command: "goland", Args: dirArg},
	{ID: "webstorm", Name: "WebStorm", Kind: Editor, App: "WebStorm", Command: "webstorm", Args: dirArg},
	{ID: "pycharm", Name: "PyCharm", Kind: Editor, App: "PyCharm", Command: "pycharm", Args: dirArg},
	{ID: "android-studio", Name: "Android Studio", Kind: Editor, App: "Android Studio"},
	{ID: "tower", Name: "Tower", Kind: Editor, App: "Tower"},

	// Git clients are found only as macOS applications.
	{ID: "fork", Name: "Fork", Kind: Git, App: "Fork"},
}

// Kinds lists the kinds in the order a window shows their buttons.
var Kinds = []string{Files, Terminal, Editor, Git}

// Installed is a launcher found on this computer.
type Installed struct {
	Launcher
	// Path is the application bundle on macOS, otherwise the executable.
	Path string
	// Icon is the application's icon as a PNG, nil where it has none.
	Icon []byte
}

// ByID finds a launcher of the catalog.
func ByID(id string) (Launcher, bool) {
	for _, l := range Catalog {
		if l.ID == id {
			return l, true
		}
	}
	return Launcher{}, false
}

// Normalize keeps known choices of kind, including ones not installed on
// this computer, and resets anything else to automatic.
func Normalize(id, kind string) string {
	if id == None {
		return id
	}
	if l, ok := ByID(id); ok && l.Kind == kind {
		return id
	}
	return ""
}

// Resolve finds choice among installed launchers of kind. An empty or
// uninstalled choice falls back to the first one installed; None has none.
func Resolve(installed []Installed, kind, choice string) *Installed {
	if choice == None {
		return nil
	}
	var first *Installed
	for i := range installed {
		l := &installed[i]
		if l.Kind != kind {
			continue
		}
		if l.ID == choice {
			return l
		}
		if first == nil {
			first = l
		}
	}
	return first
}

// Detect finds this computer's launchers with Find, using the process's
// environment and, on macOS, its application folders. It performs file I/O
// and runs plutil, so callers run it off the UI thread.
func Detect() []Installed {
	var dirs []string
	if runtime.GOOS == "darwin" {
		dirs = MacAppDirs()
	}
	return Find(runtime.GOOS, os.Environ(), dirs)
}

// Find looks for installed launchers and their icons: application bundles in
// appDirs on darwin, otherwise commands on env's PATH.
func Find(goos string, env []string, appDirs []string) []Installed {
	var found []Installed
	for _, l := range Catalog {
		if goos == "darwin" {
			if l.App == "" {
				continue
			}
			for _, dir := range appDirs {
				bundle := filepath.Join(dir, l.App+".app")
				if info, err := os.Stat(bundle); err == nil && info.IsDir() {
					found = append(found, Installed{Launcher: l, Path: bundle, Icon: bundleIcon(bundle)})
					break
				}
			}
			continue
		}
		if l.Command == "" {
			continue
		}
		if path := lookPath(env, l.Command, goos); path != "" {
			found = append(found, Installed{Launcher: l, Path: path})
		}
	}
	return found
}

// MacAppDirs lists the folders macOS applications are installed in.
func MacAppDirs() []string {
	dirs := []string{"/Applications", "/Applications/Utilities", "/System/Applications", "/System/Applications/Utilities",
		"/System/Library/CoreServices"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "Applications"))
	}
	return dirs
}

// lookPath searches env's PATH, which may be a login shell's that desktop
// launches lack.
func lookPath(env []string, name, goos string) string {
	extensions := []string{""}
	if goos == "windows" {
		extensions = []string{".exe", ".cmd", ".bat"}
	}
	for i := len(env) - 1; i >= 0; i-- {
		key, value, ok := strings.Cut(env[i], "=")
		if !ok || !strings.EqualFold(key, "PATH") {
			continue
		}
		for _, dir := range filepath.SplitList(value) {
			for _, ext := range extensions {
				path := filepath.Join(dir, name+ext)
				if info, err := os.Stat(path); err == nil && !info.IsDir() {
					return path
				}
			}
		}
		return ""
	}
	return ""
}

// Command opens dirs of this computer with the launcher, all of them in one
// application on macOS and only the first elsewhere. Editors' console shims
// start hidden on Windows; file managers and terminals keep their windows.
func (l *Installed) Command(dirs ...string) *exec.Cmd {
	if runtime.GOOS == "darwin" {
		return exec.Command("/usr/bin/open", append([]string{"-a", l.Path}, dirs...)...)
	}
	dir := dirs[0]
	cmd := exec.Command(l.Path, l.Args(dir)...)
	cmd.Dir = dir
	if l.Kind == Editor {
		childproc.HideConsole(cmd)
	}
	return cmd
}

// Run starts cmd and waits for it, which only matters on macOS, where open
// reports an application that failed to open the directory.
func Run(cmd *exec.Cmd) error {
	err := cmd.Start()
	if err == nil {
		err = cmd.Wait()
		if runtime.GOOS != "darwin" {
			err = nil
		}
	}
	return err
}

// bundleIcon decodes the application's icon from its .icns resource.
func bundleIcon(bundle string) []byte {
	name := "AppIcon"
	if out, err := exec.Command("/usr/bin/plutil", "-extract", "CFBundleIconFile", "raw", "-o", "-",
		filepath.Join(bundle, "Contents", "Info.plist")).Output(); err == nil && strings.TrimSpace(string(out)) != "" {
		name = strings.TrimSpace(string(out))
	}
	if filepath.Ext(name) != ".icns" {
		name += ".icns"
	}
	data, err := os.ReadFile(filepath.Join(bundle, "Contents", "Resources", name))
	if err != nil {
		return nil
	}
	return icnsPNG(data)
}

// icnsPNG returns the smallest PNG representation that stays sharp at menu
// size on high-density displays.
func icnsPNG(data []byte) []byte {
	if len(data) < 8 || string(data[:4]) != "icns" {
		return nil
	}
	images := map[string][]byte{}
	for i := 8; i+8 <= len(data); {
		size := int(binary.BigEndian.Uint32(data[i+4 : i+8]))
		if size < 8 || i+size > len(data) {
			break
		}
		if body := data[i+8 : i+size]; bytes.HasPrefix(body, []byte("\x89PNG")) {
			images[string(data[i:i+4])] = body
		}
		i += size
	}
	// 64, 128, 64 (32@2x), 256 (128@2x), 256, 512 (256@2x), 512, 1024.
	for _, kind := range []string{"ic12", "ic07", "ic11", "ic13", "ic08", "ic14", "ic09", "ic10"} {
		if png := images[kind]; png != nil {
			return png
		}
	}
	return nil
}
