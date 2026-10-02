package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/timbrinded/girdle/internal/kernel"
)

// Resume lets the TUI carry on the conversations whose event logs are in
// LogDir: /resume lists those that ran in the session's directory.
type Resume struct {
	LogDir string
	// Start is the conversation to carry on at start, if any.
	Start *kernel.Saved
}

// openConversations opens the picker on the earlier conversations in this
// directory, newest first.
func (m *model) openConversations(query string) tea.Cmd {
	if m.running {
		m.say(toneNotice, "/resume waits until this request ends; ctrl+c stops it")
		return nil
	}
	convs, err := kernel.Conversations(m.logDir, m.dir)
	if err != nil {
		m.say(toneError, "listing conversations: "+err.Error())
		return nil
	}
	m.convs = slices.DeleteFunc(convs, func(c kernel.Conversation) bool { return c.ID == m.sess.ID })
	if len(m.convs) == 0 {
		m.say(toneInfo, "no earlier conversation in "+tilde(m.dir)+" to carry on")
		return nil
	}
	return m.openPicker(pickConversation, query)
}

// conversationRows lists the conversations matching query: by recency
// without one, by how well their first request matches with one.
func (m *model) conversationRows(query string) []row {
	var rows []row
	if query == "" {
		for _, c := range m.convs {
			rows = append(rows, row{id: c.ID})
		}
		return rows
	}
	cands := make([]candidate, len(m.convs))
	for i, c := range m.convs {
		// Newer conversations win ties.
		cands[i] = candidate{id: c.ID, name: c.Title, created: c.Updated.Unix()}
	}
	for _, c := range search(query, cands) {
		rows = append(rows, row{id: c.id})
	}
	return rows
}

// listed is the listed conversation with id.
func (m *model) listed(id string) (kernel.Conversation, bool) {
	i := slices.IndexFunc(m.convs, func(c kernel.Conversation) bool { return c.ID == id })
	if i < 0 {
		return kernel.Conversation{}, false
	}
	return m.convs[i], true
}

// resume swaps the session for one carrying on c, in c's own log, with the
// model and effort chosen here.
func (m *model) resume(c kernel.Conversation) bool {
	saved, err := kernel.LoadConversation(c)
	if err != nil {
		m.notify(toneError, "can't carry that conversation on: "+err.Error())
		return false
	}
	log, err := kernel.OpenLog(c.Log)
	if err != nil {
		m.notify(toneError, "opening its event log: "+err.Error())
		return false
	}
	m.logs = append(m.logs, log)
	cfg := m.cfg
	cfg.Log = log
	m.sess = kernel.ResumeSession(cfg, saved)
	m.sess.Configure(m.settings)
	m.logPath = c.Log
	m.blocks, m.status, m.usage, m.routed = nil, "", "", ""
	m.replay(saved)
	return true
}

// replay shows a saved conversation as it was shown when it ran.
func (m *model) replay(saved kernel.Saved) {
	for _, e := range saved.Events {
		if e.Type == kernel.EventUserMessage {
			m.add(&block{kind: blockUser, text: e.Text})
			m.runStart = e.Time
			continue
		}
		m.show(e)
	}
	// A run cut short left calls that never finished.
	for _, b := range m.blocks {
		if b.kind == blockTool && !b.finished {
			b.finished = true
			b.invalidate()
		}
	}
	m.add(&block{kind: blockNote, tone: toneInfo, text: fmt.Sprintf("carrying on the conversation from %s · %s", ago(saved.Updated), tilde(saved.Log))})
	m.refresh()
	m.vp.GotoBottom()
}

// ago says how long since t, roughly.
func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

// closeLogs closes the event logs opened by /resume.
func (m *model) closeLogs() {
	for _, l := range m.logs {
		l.Close()
	}
}

// conversationAbout is a conversation's row summary.
func conversationAbout(c kernel.Conversation) string {
	return strings.Join([]string{ago(c.Updated), c.ID[:min(8, len(c.ID))]}, " · ")
}
