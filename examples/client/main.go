// Command client shows the conversations of an agent server, such as
// examples/server, at AGIO_SERVER (default 127.0.0.1:7788) with the token
// AGIO_TOKEN. Run it with go tool nsgo dev examples/client.
package main

import (
	"log"
	"os"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/chatview"
	"github.com/oneo-me/nsgo-agio/wire"
)

func main() {
	addr := os.Getenv("AGIO_SERVER")
	if addr == "" {
		addr = "127.0.0.1:7788"
	}
	c, err := wire.Dial(addr, os.Getenv("AGIO_TOKEN"))
	if err != nil {
		log.Fatalf("connect to %s: %v", addr, err)
	}
	nsgo.Run(func(cx *core.Scope) core.Node {
		return chatview.App{Model: chatview.Bind(cx, c), Name: agent.Client.Title}
	}, nsgo.WindowSize(1040, 720))
	// Closing the window leaves the server's agents running.
	c.Close()
}
