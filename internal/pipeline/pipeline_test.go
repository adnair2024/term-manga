package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ashwinnair/term-manga/internal/source"
)

func TestGroupVolumes(t *testing.T) {
	var chs []source.Chapter
	for _, c := range []struct{ n, v string }{{"1", "1"}, {"2", "1"}, {"3", "2"}, {"4", ""}, {"5", ""}, {"6", ""}} {
		chs = append(chs, source.Chapter{ID: c.n, Number: c.n, Volume: c.v})
	}
	got := GroupVolumes(chs, 2)
	var labels []string
	for _, v := range got {
		labels = append(labels, v.Label)
	}
	want := []string{"Vol. 1", "Vol. 2", "Ch. 4-5", "Ch. 6"}
	if len(labels) != len(want) {
		t.Fatalf("got %v", labels)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("got %v want %v", labels, want)
		}
	}
}

func TestGroupVolumesPartsStayBelowNextVolume(t *testing.T) {
	var chs []source.Chapter
	for i := 0; i < 30; i++ {
		chs = append(chs, source.Chapter{ID: string(rune('a' + i)), Number: "1", Volume: "3"})
	}
	for _, v := range GroupVolumes(chs, 2) {
		if v.Index < 3 || v.Index >= 4 {
			t.Fatalf("%s index %v leaks outside volume 3", v.Label, v.Index)
		}
	}
}

// An oversized EPUB left from an earlier run must be rejected and deleted, not
// reused, so Ingest never sees it.
func TestConvertMergedRejectsOversizedExisting(t *testing.T) {
	out := filepath.Join(t.TempDir(), "epub", "big.epub")
	os.MkdirAll(filepath.Dir(out), 0o755)
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate((MaxEpubMB + 1) << 20); err != nil { // sparse file
		t.Fatal(err)
	}
	f.Close()
	if err := ConvertMerged(context.Background(), nil, "t", out); err == nil {
		t.Fatal("expected size error")
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("oversized EPUB was not deleted")
	}
}
