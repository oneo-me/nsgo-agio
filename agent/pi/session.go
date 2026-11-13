package pi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
)

// SessionHistory is the active branch of a saved session.
type SessionHistory struct {
	Name     string
	Messages []Message
}

// ReadSession reads a session JSONL file and returns the messages from the
// root to the current leaf, the last appended entry. Abandoned branches and
// metadata entries are omitted; malformed lines are skipped.
func ReadSession(path string) (SessionHistory, error) {
	f, err := os.Open(path)
	if err != nil {
		return SessionHistory{}, err
	}
	defer f.Close()
	return parseSession(f)
}

func parseSession(r io.Reader) (SessionHistory, error) {
	type entry struct {
		Type     string   `json:"type"`
		ID       string   `json:"id"`
		ParentID *string  `json:"parentId"`
		Message  *Message `json:"message"`
		Name     *string  `json:"name"`
	}
	var history SessionHistory
	byID := map[string]*entry{}
	leaf := ""
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := br.ReadBytes('\n')
		if line = bytes.TrimSpace(line); len(line) > 0 {
			var e entry
			if json.Unmarshal(line, &e) == nil && e.ID != "" && e.Type != "session" {
				byID[e.ID] = &e
				leaf = e.ID
				if e.Type == "session_info" && e.Name != nil {
					history.Name = *e.Name
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return history, err
		}
	}
	var chain []*entry
	for id, seen := leaf, map[string]bool{}; id != "" && !seen[id]; {
		seen[id] = true
		e := byID[id]
		if e == nil {
			break
		}
		chain = append(chain, e)
		if e.ParentID == nil {
			break
		}
		id = *e.ParentID
	}
	for i := len(chain) - 1; i >= 0; i-- {
		if e := chain[i]; e.Type == "message" && e.Message != nil {
			history.Messages = append(history.Messages, *e.Message)
		}
	}
	return history, nil
}
