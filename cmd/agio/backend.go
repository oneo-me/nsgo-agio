//go:build !agiodemo

package main

import (
	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/host"
	"github.com/oneo-me/nsgo-agio/quota"
)

// shownFirst is the task the window opens on, "" for New task.
const shownFirst = ""

// openBackend runs the agents in this process, as the settings left them,
// with the signed-in accounts' limits; a demo build shows a scripted one
// instead (demo.go).
func openBackend(dir, home string, s settings) agio.Backend {
	return host.New(host.Options{Dir: dir, Disabled: s.Disabled, DisabledModels: s.DisabledModels,
		ReadQuota: quota.Reader(home)})
}
