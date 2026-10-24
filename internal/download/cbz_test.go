package download

import (
	"archive/zip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCBZ(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("img" + r.URL.Path)) }))
	defer srv.Close()
	dest := ChapterPath(t.TempDir(), "My: Series", "1.5")
	err := CBZ(context.Background(), dest, []string{srv.URL + "/a.png", srv.URL + "/b.jpg"}, nil)
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
	if err := CBZ(ctx, dest, []string{srv.URL + "/a.png"}, nil); err == nil {
		t.Fatal("expected error")
	}
	for _, p := range []string{dest, dest + ".part"} {
		if _, err := os.Stat(p); err == nil {
			t.Fatal("left behind", p)
		}
	}
}
