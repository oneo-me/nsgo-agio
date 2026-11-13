package launchers

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func icnsEntry(kind string, body []byte) []byte {
	entry := make([]byte, 8, 8+len(body))
	copy(entry, kind)
	binary.BigEndian.PutUint32(entry[4:], uint32(8+len(body)))
	return append(entry, body...)
}

func testPNG(size int) []byte {
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, size, size)))
	return b.Bytes()
}

func testICNS() []byte {
	body := append(icnsEntry("it32", []byte("legacy")), icnsEntry("ic08", testPNG(4))...)
	body = append(body, icnsEntry("ic07", testPNG(2))...)
	header := make([]byte, 8)
	copy(header, "icns")
	binary.BigEndian.PutUint32(header[4:], uint32(8+len(body)))
	return append(header, body...)
}

func TestICNSPrefersSmallSharpPNG(t *testing.T) {
	data := testICNS()
	if got := icnsPNG(data); !bytes.Equal(got, testPNG(2)) {
		t.Fatal("did not choose the 128 px representation")
	}
	if icnsPNG([]byte("icns\x00\x00\x00\x08")) != nil || icnsPNG([]byte("nope")) != nil || icnsPNG(data[:20]) != nil {
		t.Fatal("malformed icon data accepted")
	}
}

func TestFind(t *testing.T) {
	apps := t.TempDir()
	resources := filepath.Join(apps, "Ghostty.app", "Contents", "Resources")
	os.MkdirAll(resources, 0755)
	os.WriteFile(filepath.Join(resources, "AppIcon.icns"), testICNS(), 0644)
	os.MkdirAll(filepath.Join(apps, "Zed.app"), 0755)
	os.MkdirAll(filepath.Join(apps, "Finder.app"), 0755)
	os.WriteFile(filepath.Join(apps, "Cursor.app"), nil, 0644) // not a bundle

	found := Find("darwin", nil, []string{t.TempDir(), apps})
	if len(found) != 3 || found[0].ID != "finder" || found[0].Kind != Files ||
		found[1].ID != "ghostty" || found[1].Icon == nil || found[2].ID != "zed" || found[2].Icon != nil {
		t.Fatalf("mac launchers: %+v", found)
	}

	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "code"), nil, 0755)
	os.WriteFile(filepath.Join(bin, "wezterm"), nil, 0755)
	os.WriteFile(filepath.Join(bin, "xdg-open"), nil, 0755)
	found = Find("linux", []string{"PATH=/nowhere", "PATH=" + bin}, nil)
	if len(found) != 3 || found[0].ID != "xdg-open" || found[1].ID != "wezterm" || found[2].ID != "vscode" || found[2].Path != filepath.Join(bin, "code") {
		t.Fatalf("linux launchers: %+v", found)
	}
	if args := found[1].Args("/w"); len(args) != 3 || args[2] != "/w" {
		t.Fatalf("wezterm arguments: %v", args)
	}
}

func TestResolveAndNormalize(t *testing.T) {
	installed := []Installed{
		{Launcher: Launcher{ID: "ghostty", Kind: Terminal}},
		{Launcher: Launcher{ID: "vscode", Kind: Editor}},
		{Launcher: Launcher{ID: "zed", Kind: Editor}},
	}
	if l := Resolve(installed, Editor, "zed"); l == nil || l.ID != "zed" {
		t.Fatalf("chosen editor: %+v", l)
	}
	if l := Resolve(installed, Editor, "cursor"); l == nil || l.ID != "vscode" {
		t.Fatalf("uninstalled choice falls back to the first editor: %+v", l)
	}
	if Resolve(installed, Editor, None) != nil || Resolve(installed, Files, "") != nil {
		t.Fatal("hidden or missing kind resolved")
	}
	if Normalize("zed", Editor) != "zed" || Normalize("zed", Terminal) != "" || Normalize("nope", Editor) != "" || Normalize(None, Editor) != None {
		t.Fatal("choices normalized wrongly")
	}
}
