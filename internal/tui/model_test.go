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
	if v := m.View(); !strings.Contains(v, "merging Vol. 1") {
		t.Fatalf("stage missing from view:\n%s", v)
	}
}
