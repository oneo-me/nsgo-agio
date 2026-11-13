package main

import (
	"encoding/binary"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/cmd/agio/locales"
	"github.com/oneo-me/nsgo-agio/transcript"
)

// alerts plays a sound and shows a notification, as the settings prefer,
// for every conversation whose agent stopped working since the last look.
// One the user stopped raises none.
type alerts struct {
	working map[string]bool
	// play and notify are playTaskSound and showTaskNotification but in
	// tests.
	play   func(id string, failed bool)
	notify func(taskNotification)
}

// look compares the conversations with the last look; lang is the language
// to say the notification in.
func (al *alerts) look(s settings, list []*agio.Conversation, lang string) {
	working := map[string]bool{}
	for _, c := range list {
		if c.Running {
			working[c.ID] = true
			continue
		}
		if !al.working[c.ID] {
			continue
		}
		status := transcript.Completed
		if n := len(c.Turns); n > 0 {
			status = c.Turns[n-1].Status
		}
		if status == transcript.Cancelled {
			continue
		}
		al.alert(s, c, status == transcript.Failed || c.Err != "", lang)
	}
	al.working = working
}

func (al *alerts) alert(s settings, c *agio.Conversation, failed bool, lang string) {
	if s.TaskSound != "" {
		play := al.play
		if play == nil {
			play = playTaskSound
		}
		play(s.TaskSound, failed)
	}
	if s.TaskNotification {
		msgs := locales.In(lang)
		body := msgs.TaskCompleted()
		if failed {
			body = msgs.TaskFailed()
		}
		title := c.Title
		if title == "" {
			title = msgs.Untitled()
		}
		notify := al.notify
		if notify == nil {
			notify = showTaskNotification
		}
		// The sound preference replaces the system's notification sound.
		notify(taskNotification{Title: title, Body: body, Silent: s.TaskSound != ""})
	}
}

// taskNotification is a system notification about a conversation.
type taskNotification struct {
	Title, Body string
	Silent      bool
}

// showTaskNotification asks the system to show a notification off the main
// thread, through the command each system has.
func showTaskNotification(n taskNotification) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("/usr/bin/osascript", "-e", "on run argv", "-e", "display notification (item 2 of argv) with title (item 1 of argv)", "-e", "end run", n.Title, n.Body)
	case "linux":
		cmd = exec.Command("notify-send", n.Title, n.Body)
	default:
		return
	}
	go func() {
		if err := cmd.Run(); err != nil {
			log.Printf("show notification: %v", err)
		}
	}()
}

// taskSound is one choice of alert sound: a completed task plays its rising
// version and a failed one its falling version. The sounds are synthesized
// rather than shipped, and each is rendered the first time it plays.
type taskSound struct {
	id                string
	completed, failed func() string
}

// taskSounds lists the choices in menu order, after none.
var taskSounds = []taskSound{
	{"chime", melody(bell, 0.12, 0.6, 880, 1318.51), melody(bell, 0.12, 0.6, 659.25, 440)},
	{"glass", melody(glass, 0, 1.2, 1567.98), melody(glass, 0, 1.2, 739.99)},
	{"marimba", melody(marimba, 0.09, 0.4, 1046.5, 1318.51, 1567.98), melody(marimba, 0.09, 0.4, 783.99, 659.25, 523.25)},
	{"pop", melody(pop, 0.08, 0.15, 1046.5, 1396.91), melody(pop, 0.08, 0.15, 698.46, 523.25)},
	{"digital", melody(beep, 0.13, 0.1, 1760, 1760), melody(beep, 0.13, 0.1, 440, 349.23)},
}

func taskSoundIDs() []string {
	ids := make([]string, len(taskSounds))
	for i, s := range taskSounds {
		ids[i] = s.id
	}
	return ids
}

func findTaskSound(id string) (taskSound, bool) {
	for _, s := range taskSounds {
		if s.id == id {
			return s, true
		}
	}
	return taskSound{}, false
}

func playTaskSound(id string, failed bool) {
	s, ok := findTaskSound(id)
	if !ok {
		return
	}
	file := s.completed
	if failed {
		file = s.failed
	}
	go playFile(file())
}

// playFile plays a sound file with the command each system has.
func playFile(path string) {
	if path == "" {
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("/usr/bin/afplay", path)
	case "linux":
		cmd = exec.Command("paplay", path)
	case "windows":
		cmd = exec.Command("powershell", "-NoProfile", "-Command", "(New-Object Media.SoundPlayer '"+path+"').PlaySync()")
	default:
		return
	}
	if err := cmd.Run(); err != nil {
		log.Printf("play sound: %v", err)
	}
}

// voice is the sample of an instrument playing freq, t seconds after the
// note began.
type voice func(freq, t float64) float64

// melody plays freqs one step apart, letting the last note ring for tail
// seconds, and scales the result to one peak level so every choice is
// equally loud.
func melody(v voice, step, tail float64, freqs ...float64) func() string {
	return sync.OnceValue(func() string {
		const rate, peak = 48000, 0.35
		samples := make([]int16, int(rate*(step*float64(len(freqs)-1)+tail)))
		values := make([]float64, len(samples))
		loudest := 0.0
		for i := range values {
			t := float64(i) / rate
			for n, freq := range freqs {
				if at := step * float64(n); t >= at {
					values[i] += v(freq, t-at)
				}
			}
			loudest = max(loudest, math.Abs(values[i]))
		}
		for i, value := range values {
			samples[i] = int16(peak * value / max(loudest, 1e-9) * math.MaxInt16)
		}
		return writeWAV(rate, samples)
	})
}

// writeWAV writes 16-bit mono samples to a file of its own, returning its
// path, or "" when it could not.
func writeWAV(rate uint32, samples []int16) string {
	f, err := os.CreateTemp("", "agio-sound-*.wav")
	if err != nil {
		return ""
	}
	defer f.Close()
	size := uint32(2 * len(samples))
	le := binary.LittleEndian
	head := []byte("RIFF\x00\x00\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00")
	head = le.AppendUint32(head, rate)
	head = le.AppendUint32(head, rate*2)
	head = append(head, 2, 0, 16, 0, 'd', 'a', 't', 'a')
	head = le.AppendUint32(head, size)
	le.PutUint32(head[4:], size+uint32(len(head))-8)
	data := head
	for _, sample := range samples {
		data = le.AppendUint16(data, uint16(sample))
	}
	if _, err := f.Write(data); err != nil {
		return ""
	}
	return filepath.Clean(f.Name())
}

func sine(freq, t float64) float64 { return math.Sin(2 * math.Pi * freq * t) }

// attack ramps a note in over 5 ms, so it does not click.
func attack(t float64) float64 { return math.Min(1, t/0.005) }

// bell is a note with an octave overtone and an exponential decay.
func bell(freq, t float64) float64 {
	return attack(t) * math.Exp(-t*7) * (sine(freq, t) + 0.3*sine(2*freq, t))
}

// glass rings long with the inharmonic partials of a struck glass.
func glass(freq, t float64) float64 {
	return attack(t) * math.Exp(-t*3.5) * (sine(freq, t) + 0.4*sine(2.76*freq, t)*math.Exp(-t*3) + 0.2*sine(5.4*freq, t)*math.Exp(-t*6))
}

// marimba is a short wooden note whose bright overtone fades first.
func marimba(freq, t float64) float64 {
	return attack(t) * math.Exp(-t*14) * (sine(freq, t) + 0.25*sine(4*freq, t)*math.Exp(-t*30))
}

// pop starts above its pitch and drops to it quickly, like a bubble.
func pop(freq, t float64) float64 {
	const bend, speed = 0.6, 60
	phase := 2 * math.Pi * freq * (t + bend*(1-math.Exp(-speed*t))/speed)
	return attack(t) * math.Exp(-t*25) * math.Sin(phase)
}

// beep is a short, softened square wave.
func beep(freq, t float64) float64 {
	const length = 0.09
	envelope := math.Max(0, math.Min(attack(t), (length-t)/0.01))
	return envelope * (sine(freq, t) + sine(3*freq, t)/3 + sine(5*freq, t)/5)
}
