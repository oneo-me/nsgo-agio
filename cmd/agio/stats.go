package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/cmd/agio/locales"
)

// Agio counts its use in its site's Umami, as a page of the site under
// /nsgo-agio/app/ tagged "app", as the site's own pages are "website": a
// pageview when it opens, the event "message" with the agent for each
// message sent, and heartbeats while it is used. An install carries a
// random ID kept in the settings, so that one install is one visitor.
// Nothing about prompts, folders or models is sent. Builds of nsgo dev
// (statsBuild) and test binaries report nothing, and the settings turn it
// off.
const (
	statsURL     = "https://umami.oneo.me/api/send"
	statsWebsite = "c4801ae4-210f-4877-860d-807a8b8ed0e0"
	statsHost    = "oneo-me.github.io"
	statsPath    = "/nsgo-agio/app/"
	statsTag     = "app"
)

// Umami measures a visit from its first event to its last, so Agio looks
// at its conversations each statsBeat and sends a heartbeat for statsIdle
// after a message was last sent or an agent last worked: a window left
// open does not count as use.
const (
	statsBeat = time.Minute
	statsIdle = 5 * time.Minute
)

// statsPayload is an event of Umami's /api/send: a pageview without a
// name.
type statsPayload struct {
	Website  string            `json:"website"`
	Hostname string            `json:"hostname"`
	URL      string            `json:"url"`
	Title    string            `json:"title"`
	Language string            `json:"language,omitempty"`
	Screen   string            `json:"screen,omitempty"`
	Tag      string            `json:"tag"`
	ID       string            `json:"id,omitempty"`
	Name     string            `json:"name,omitempty"`
	Data     map[string]string `json:"data,omitempty"`
}

// reportUsage sends the pageview, the messages' events and the heartbeats.
// Events are made on the graph's goroutine and sent from another, oldest
// first; one that fails is dropped.
func reportUsage(cx *core.Scope, b agio.Backend, st *store) {
	if !statsBuild || testing.Testing() {
		return
	}
	env := cx.Env()
	id := installID(st)
	queue := make(chan statsPayload, 32)
	go sendStats(queue, statsAgent(env.OS()))

	send := func(name string, data map[string]string) {
		if st.get().StatsOff {
			return
		}
		w := env.Window()
		p := statsPayload{Website: statsWebsite, Hostname: statsHost, URL: statsPath, Title: agent.Client.Title,
			Language: env.Locale(), Screen: fmt.Sprintf("%dx%d", int(w.Width), int(w.Height)),
			Tag: statsTag, ID: id, Name: name, Data: data}
		select {
		case queue <- p:
		default:
		}
	}
	send("", nil)
	looked, used := time.Now(), time.Now()
	var beat func()
	beat = func() {
		now := time.Now()
		tools, working := activity(b.State(), looked)
		looked = now
		for _, t := range tools {
			send("message", map[string]string{"agent": t.Name()})
		}
		if working || len(tools) > 0 {
			used = now
		}
		if now.Sub(used) < statsIdle {
			send("heartbeat", nil)
		}
		cx.AfterFunc(statsBeat, beat)
	}
	cx.AfterFunc(statsBeat, beat)
}

// activity returns the agents of the turns started after since, one for
// each, and whether any agent is starting or working.
func activity(s agio.State, since time.Time) (tools []agent.Tool, working bool) {
	for _, c := range s.Conversations {
		working = working || c.Starting || c.Running
		tool, _ := agent.ParseKey(c.Model)
		for i := len(c.Turns) - 1; i >= 0 && c.Turns[i].Started.After(since); i-- {
			tools = append(tools, tool)
		}
	}
	return tools, working
}

// installID returns the install's random ID, made on first use.
func installID(st *store) string {
	var id string
	st.update(func(s *settings) {
		if s.InstallID == "" {
			b := make([]byte, 16)
			rand.Read(b)
			s.InstallID = hex.EncodeToString(b)
		}
		id = s.InstallID
	})
	return id
}

// sendStats posts the queue's events to Umami until it closes.
func sendStats(queue <-chan statsPayload, agent string) {
	client := &http.Client{Timeout: 10 * time.Second}
	for p := range queue {
		body, _ := json.Marshal(map[string]any{"type": "event", "payload": p})
		req, err := http.NewRequest(http.MethodPost, statsURL, bytes.NewReader(body))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		if agent != "" {
			req.Header.Set("User-Agent", agent)
		}
		if resp, err := client.Do(req); err == nil {
			resp.Body.Close()
		}
	}
}

// statsAgent is the User-Agent Agio sends, as a browser on the platform
// would say it, for Umami to tell the system and not take it for a bot as
// it does Go's own.
func statsAgent(os core.OS) string {
	platforms := map[core.OS]string{
		core.OSMacOS:   "Macintosh; Intel Mac OS X 10_15_7",
		core.OSWindows: "Windows NT 10.0; Win64; x64",
		core.OSLinux:   "X11; Linux x86_64",
	}
	p, ok := platforms[os]
	if !ok {
		return ""
	}
	return "Mozilla/5.0 (" + p + ") Agio/1.0"
}

// statsSettings is the settings' category turning the usage statistics on
// or off.
func statsSettings(cx *core.Scope, st *store) ui.SettingsCategory {
	on := core.NewSignal(cx, !st.get().StatsOff)
	toggle := ui.Switch{Checked: on.Get, Label: locales.SettingsStatsSend(cx), OnChange: func(v bool) {
		on.Set(v)
		st.update(func(s *settings) { s.StatsOff = !v })
	}}
	return ui.SettingsCategory{ID: "privacy", Title: locales.SettingsPrivacy(cx), Icon: lucide.ChartColumn,
		Groups: []ui.SettingsGroup{{ID: "stats", Title: locales.SettingsStats(cx), Items: []ui.SettingsItem{{
			ID: "send", Title: locales.SettingsStatsSend(cx), Description: locales.SettingsStatsDetail(cx),
			Keywords: []string{"statistics", "analytics", "privacy", "telemetry", locales.SettingsStats(cx)},
			Control:  toggle,
		}}}}}
}
