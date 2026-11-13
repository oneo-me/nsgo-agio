//go:build windows

package launchers

import "testing"

// The file manager opens a window the user must see, so it must not be
// launched hidden; only editors' console shims are hidden.
func TestFileManagerKeepsItsWindow(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"explorer", "vscode"} {
		l, _ := ByID(id)
		cmd := (&Installed{Launcher: l}).Command(dir)
		hidden := cmd.SysProcAttr != nil && cmd.SysProcAttr.HideWindow
		if id == "explorer" && hidden {
			t.Fatal("the file manager would open its window hidden")
		}
		if id == "vscode" && !hidden {
			t.Fatal("an editor console would flash a window")
		}
	}
}
