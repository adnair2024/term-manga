package download

import (
	"archive/zip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestCBZ(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("img" + r.URL.Path)) }))
	defer srv.Close()
	dest := ChapterPath(t.TempDir(), "My: Series", "1.5")
	err := CBZ(context.Background(), dest, []string{srv.URL + "/a.png", srv.URL + "/b.jpg?t=1"}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dest) != "My_ Series - Ch. 1.5.cbz" {
		t.Fatal(dest)
	}
	zr, err := zip.OpenReader(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != 2 || zr.File[0].Name != "0001.png" || zr.File[1].Name != "0002.jpg" {
		t.Fatalf("%v", zr.File)
	}
	if _, err := os.Stat(dest + ".part"); err == nil {
		t.Fatal("leftover .part")
	}
}

func TestCBZFailureLeavesNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dest := filepath.Join(t.TempDir(), "x.cbz")
	if err := CBZ(ctx, dest, []string{srv.URL + "/a.png"}, "", nil); err == nil {
		t.Fatal("expected error")
	}
	for _, p := range []string{dest, dest + ".part"} {
		if _, err := os.Stat(p); err == nil {
			t.Fatal("left behind", p)
		}
	}
}

// While the first page is slow, workers must not fetch more than pageAhead pages.
func TestCBZBoundsPagesAhead(t *testing.T) {
	var served atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served.Add(1)
		if r.URL.Path == "/0.jpg" {
			<-release
		}
		w.Write([]byte("x"))
	}))
	defer srv.Close()
	var urls []string
	for i := 0; i < 60; i++ {
		urls = append(urls, srv.URL+"/"+strconv.Itoa(i)+".jpg")
	}
	done := make(chan error, 1)
	go func() { done <- CBZ(context.Background(), filepath.Join(t.TempDir(), "x.cbz"), urls, "", nil) }()
	time.Sleep(300 * time.Millisecond)
	if n := served.Load(); n > pageAhead {
		t.Fatalf("%d pages fetched while page 0 was pending, want <= %d", n, pageAhead)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
