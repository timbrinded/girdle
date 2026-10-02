package kernel

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"charm.land/fantasy"
)

// A conversation is kept in its session's event log: messages events hold
// what each request added to the history, so a later run can carry the
// conversation on from the log alone (decision 0026).

// Conversation is a session found in an event log.
type Conversation struct {
	ID    string
	Dir   string
	Title string // the first request, on one line
	Log   string // the event log's path
	// Updated is when the log last changed.
	Updated time.Time
}

// Saved is a conversation read back from its log.
type Saved struct {
	Conversation
	Messages []fantasy.Message
	// Events are the conversation's logged events, in order, to show it
	// again. Messages events are left out.
	Events []Event
}

// ErrNotResumable is returned for a conversation whose log holds no
// messages, such as one recorded before Girdle kept them.
var ErrNotResumable = errors.New("its log holds no messages to carry on from")

// Conversations lists the conversations whose logs are in logDir and that
// ran in dir, newest first. A log without a request is left out.
func Conversations(logDir, dir string) ([]Conversation, error) {
	paths, err := filepath.Glob(filepath.Join(logDir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	var out []Conversation
	for _, p := range paths {
		c, ok := readHeader(p)
		if ok && c.Dir == dir {
			out = append(out, c)
		}
	}
	slices.SortFunc(out, func(a, b Conversation) int {
		return cmp.Or(b.Updated.Compare(a.Updated), strings.Compare(a.ID, b.ID))
	})
	return out, nil
}

// FindConversation picks a conversation by its ID, or a prefix of it that
// only one has.
func FindConversation(convs []Conversation, id string) (Conversation, error) {
	var found []Conversation
	for _, c := range convs {
		if c.ID == id {
			return c, nil
		}
		if id != "" && strings.HasPrefix(c.ID, id) {
			found = append(found, c)
		}
	}
	switch len(found) {
	case 0:
		return Conversation{}, fmt.Errorf("no conversation here has the ID %q", id)
	case 1:
		return found[0], nil
	default:
		return Conversation{}, fmt.Errorf("%d conversations here have IDs starting %q: give more of it", len(found), id)
	}
}

// logLine is the part of a logged event that the header needs.
type logLine struct {
	Type    EventType         `json:"type"`
	Session string            `json:"session"`
	Text    string            `json:"text"`
	Meta    map[string]string `json:"meta"`
}

// readHeader reads a log up to its first request: the session it starts
// with, and what that session was asked first.
func readHeader(path string) (Conversation, bool) {
	f, err := os.Open(path)
	if err != nil {
		return Conversation{}, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return Conversation{}, false
	}
	c := Conversation{Log: path, Updated: info.ModTime()}
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		var l logLine
		if json.Unmarshal(line, &l) == nil {
			switch {
			case l.Type == EventSessionStart && c.ID == "":
				c.ID, c.Dir = l.Session, l.Meta["dir"]
			case l.Type == EventUserMessage && l.Session == c.ID:
				c.Title = oneLine(l.Text)
				return c, c.Title != ""
			}
		}
		if err != nil {
			return Conversation{}, false
		}
	}
}

// LoadConversation reads a conversation's messages and events back from
// its log.
func LoadConversation(c Conversation) (Saved, error) {
	raw, err := os.ReadFile(c.Log)
	if err != nil {
		return Saved{}, err
	}
	s := Saved{Conversation: c}
	for line := range bytes.Lines(raw) {
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			// A line cut short by a crash ends the log.
			if errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return Saved{}, fmt.Errorf("%s: %w", c.Log, err)
		}
		if e.Session != c.ID {
			continue
		}
		if e.Type == EventMessages {
			s.Messages = append(s.Messages, e.Messages...)
			continue
		}
		s.Events = append(s.Events, e)
	}
	if len(s.Messages) == 0 {
		return Saved{}, ErrNotResumable
	}
	return s, nil
}

// ResumeSession carries on a saved conversation. The session keeps its ID,
// so its events and messages go on in the same log when cfg.Log is that
// log. The model may not be the one that wrote the history, so the history
// is made portable.
func ResumeSession(cfg Config, saved Saved) *Session {
	return newSession(cfg, saved.ID, portable(saved.Messages))
}

// record logs the messages added to the history since it last did. Every
// request ends through end, which records, so a history rewritten as a
// request starts (applySettings) has nothing left unrecorded.
func (s *Session) record() {
	if s.recorded >= len(s.history) {
		return
	}
	e := Event{Type: EventMessages, Messages: slices.Clone(s.history[s.recorded:])}
	s.recorded = len(s.history)
	s.emitMu.Lock()
	defer s.emitMu.Unlock()
	e.Time, e.Session = time.Now(), s.ID
	if err := s.cfg.Log.Write(e); err != nil && s.cfg.Emit != nil {
		s.cfg.Emit(Event{Time: e.Time, Session: s.ID, Type: EventError, Text: "event log: " + err.Error()})
	}
}
