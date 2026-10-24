// Package pipeline runs the external tools: KCC for e-reader conversion and
// calibredb for library ingestion.
package pipeline

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ashwinnair/term-manga/internal/source"
)

// Profile is the KCC device profile (Kindle Paperwhite 5, 1236x1648).
const Profile = "KPW5"

// CheckTools reports which required binaries are missing from $PATH.
func CheckTools() error {
	var missing []string
	for _, b := range []string{"kcc-c2e", "7z", "calibredb"} {
		if _, err := exec.LookPath(b); err != nil {
			missing = append(missing, b)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing from $PATH: %s", strings.Join(missing, ", "))
	}
	return nil
}

// EpubPath returns where Convert writes the EPUB for a given .cbz.
func EpubPath(cbz string) string {
	base := strings.TrimSuffix(filepath.Base(cbz), ".cbz") + ".epub"
	return filepath.Join(filepath.Dir(cbz), "epub", base)
}

// Convert runs kcc-c2e on cbz (manga mode, auto-crop, EPUB) and returns the
// EPUB path. An existing EPUB is reused.
func Convert(ctx context.Context, cbz, title string) (string, error) {
	out := EpubPath(cbz)
	if _, err := os.Stat(out); err == nil {
		return out, nil
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "kcc-c2e",
		"-p", Profile, "-m", "-c", "2", "-f", "EPUB", "--nokepub",
		"-t", title, "-o", out, cbz)
	if b, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("kcc-c2e: %w: %s", err, tail(b))
	}
	if _, err := os.Stat(out); err != nil {
		return "", fmt.Errorf("kcc-c2e finished but %s was not created", out)
	}
	return out, nil
}

// MaxVolumeChapters caps how many chapters go into one merged EPUB, and
// MaxEpubMB is the size target. Big omnibuses make Kindles lag, so merging is
// one MangaDex volume per file, with oversized ones split into parts.
const (
	MaxVolumeChapters = 12
	MaxEpubMB         = 200
)

// Volume is a run of chapters that is merged into a single EPUB.
type Volume struct {
	Label    string  // e.g. "Vol. 3" or "Ch. 41-50"
	Index    float64 // Calibre series index
	Chapters []source.Chapter
}

// GroupVolumes splits chapters (in reading order) into volumes. Consecutive
// chapters sharing a MangaDex volume stay together; chapters with no volume
// are chunked max at a time. Anything longer than max is split into parts.
func GroupVolumes(chs []source.Chapter, max int) []Volume {
	var out []Volume
	for i := 0; i < len(chs); {
		j := i
		for j < len(chs) && chs[j].Volume == chs[i].Volume {
			j++
		}
		run, vol := chs[i:j], chs[i].Volume
		i = j
		n := (len(run) + max - 1) / max
		per := (len(run) + n - 1) / n
		for p := 0; p < n; p++ {
			part := run[p*per : min(len(run), (p+1)*per)]
			v := Volume{Chapters: part}
			if vol != "" {
				v.Label = "Vol. " + vol
				v.Index, _ = strconv.ParseFloat(vol, 64)
				if n > 1 {
					v.Label += fmt.Sprintf(" pt. %d", p+1)
					v.Index += float64(p) / 10
				}
			} else {
				first, last := part[0].Number, part[len(part)-1].Number
				v.Label = "Ch. " + first
				if last != first {
					v.Label += "-" + last
				}
				v.Index, _ = strconv.ParseFloat(first, 64)
			}
			out = append(out, v)
		}
	}
	return out
}

// MergedPath returns where ConvertMerged writes the EPUB for a volume.
func MergedPath(dir, series, label string) string {
	return filepath.Join(dir, "epub", fmt.Sprintf("%s - %s.epub", series, label))
}

// ConvertMerged fuses cbzs into one EPUB at out via kcc-c2e --filefusion. An
// existing EPUB is reused.
func ConvertMerged(ctx context.Context, cbzs []string, title, out string) error {
	if _, err := os.Stat(out); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	args := []string{"-p", Profile, "-m", "-c", "2", "-f", "EPUB", "--nokepub",
		"--filefusion", "-t", title, "-o", out}
	cmd := exec.CommandContext(ctx, "kcc-c2e", append(args, cbzs...)...)
	if b, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("kcc-c2e: %w: %s", err, tail(b))
	}
	fi, err := os.Stat(out)
	if err != nil {
		return fmt.Errorf("kcc-c2e finished but %s was not created", out)
	}
	if mb := fi.Size() >> 20; mb > MaxEpubMB {
		return fmt.Errorf("%s is %d MB (over %d MB); merge fewer chapters", filepath.Base(out), mb, MaxEpubMB)
	}
	return nil
}

// Ingest adds epub to the default Calibre library. calibredb skips books whose
// title and author already exist, so re-running is safe.
func Ingest(ctx context.Context, epub, title, series, number string) error {
	args := []string{"add", "-t", title, "-a", "MangaDex", "-s", series, "-T", "manga"}
	if n, err := strconv.ParseFloat(number, 64); err == nil {
		args = append(args, "-S", strconv.FormatFloat(n, 'f', -1, 64))
	}
	args = append(args, epub)
	if b, err := exec.CommandContext(ctx, "calibredb", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("calibredb: %w: %s", err, tail(b))
	}
	return nil
}

func tail(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = "..." + s[len(s)-300:]
	}
	return s
}
