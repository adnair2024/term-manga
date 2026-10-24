// Package download fetches chapter pages into .cbz archives.
package download

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var unsafeChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

// SafeName makes s usable as a file or directory name.
func SafeName(s string) string {
	s = strings.TrimSpace(unsafeChars.ReplaceAllString(s, "_"))
	s = strings.Trim(s, ". ")
	if s == "" {
		return "untitled"
	}
	return s
}

// Root returns ~/Manga.
func Root() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Manga"), nil
}

// ChapterPath returns ~/Manga/<series>/<series> - Ch. <num>.cbz.
func ChapterPath(root, series, number string) string {
	s := SafeName(series)
	return filepath.Join(root, s, fmt.Sprintf("%s - Ch. %s.cbz", s, SafeName(number)))
}

// pageWorkers is how many pages are fetched at once. MangaDex asks clients to
// stay modest, so this is deliberately well under its rate limits.
const pageWorkers = 6

var httpClient = &http.Client{
	Timeout: 60 * time.Second,
	Transport: &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConns:        pageWorkers * 2,
		MaxIdleConnsPerHost: pageWorkers * 2, // default is 2, which forces reconnects
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	},
}

// CBZ downloads pageURLs into a zip at dest, calling progress(done, total) after
// each page. The archive is written to a temp file and renamed on success, so a
// failed download never leaves a partial .cbz behind.
func CBZ(ctx context.Context, dest string, pageURLs []string, progress func(done, total int)) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(tmp)
		}
	}()

	// Fetch pages concurrently, but write them to the zip in page order.
	fetchCtx, stop := context.WithCancel(ctx)
	defer stop()
	results := make([]chan pageResult, len(pageURLs))
	jobs := make(chan int)
	for i := range results {
		results[i] = make(chan pageResult, 1)
	}
	for w := 0; w < min(pageWorkers, len(pageURLs)); w++ {
		go func() {
			for i := range jobs {
				data, err := fetchPage(fetchCtx, pageURLs[i])
				results[i] <- pageResult{data, err}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for i := range pageURLs {
			select {
			case jobs <- i:
			case <-fetchCtx.Done():
				return
			}
		}
	}()

	zw := zip.NewWriter(f)
	for i, u := range pageURLs {
		var r pageResult
		select {
		case r = <-results[i]:
		case <-ctx.Done(): // producer stops dispatching once cancelled
			return ctx.Err()
		}
		if r.err != nil {
			return fmt.Errorf("page %d: %w", i+1, r.err)
		}
		// Images are already compressed; store them as-is, zero-padded for page order.
		w, err := zw.CreateHeader(&zip.FileHeader{
			Name:   fmt.Sprintf("%04d%s", i+1, path.Ext(u)),
			Method: zip.Store,
		})
		if err != nil {
			return err
		}
		if _, err := w.Write(r.data); err != nil {
			return err
		}
		if progress != nil {
			progress(i+1, len(pageURLs))
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	ok = true
	return nil
}

type pageResult struct {
	data []byte
	err  error
}

func fetchPage(ctx context.Context, u string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "mangapipeline/0.1")
		resp, err := httpClient.Do(req)
		if err == nil {
			if resp.StatusCode == http.StatusOK {
				data, rerr := io.ReadAll(resp.Body)
				resp.Body.Close()
				if rerr == nil {
					return data, nil
				}
				err = rerr
			} else {
				resp.Body.Close()
				err = fmt.Errorf("%s", resp.Status)
			}
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
	return nil, lastErr
}
