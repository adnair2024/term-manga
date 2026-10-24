package tui

import (
	"strings"
	"testing"

	"github.com/ashwinnair/term-manga/internal/source"
)

// After the last chapter downloads in merge mode, qIdx == len(queue) while the
// merge runs. Rendering that state must not index past the queue.
func TestViewWhileMerging(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	m.screen = screenDownload
	m.queue = []source.Chapter{{ID: "a", Number: "1"}}
	m.qIdx = len(m.queue)
	m.stage = "merging Vol. 1"
	m.mergeDone, m.mergeTotal = 3, 41
	if v := m.View(); !strings.Contains(v, "merging Vol. 1") {
		t.Fatalf("stage missing from view:\n%s", v)
	} else if !strings.Contains(v, "Volumes 3/41") {
		t.Fatalf("merge progress missing from view:\n%s", v)
	}
}

func TestRecentLogTrims(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		m.log = append(m.log, "line")
	}
	if got := strings.Count(m.recentLog(), "line"); got != 10 {
		t.Fatalf("showed %d lines, want 10", got)
	}
	if !strings.Contains(m.recentLog(), "90 earlier") {
		t.Fatal("missing hidden-count note")
	}
}
