// Command single is a whole agent application in one process: the window
// and the coding agents it runs, which work in AGIO_DIR, else the home
// directory. Run it with go tool nsgo dev examples/single.
package main

import (
	"os"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/oneo-me/nsgo-agio/chatview"
	"github.com/oneo-me/nsgo-agio/host"
)

func main() {
	dir := os.Getenv("AGIO_DIR")
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	h := host.New(host.Options{Dir: dir})
	nsgo.Run(func(cx *core.Scope) core.Node {
		return chatview.App{Model: chatview.Bind(cx, h)}
	}, nsgo.WindowSize(1040, 720))
	// The window closed: its agents end with it.
	h.Close()
}
