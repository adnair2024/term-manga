package mangapill

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestScrape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/search"):
			w.Write([]byte(`<a href="/manga/1/a-b" class="mb-2">
			 <div class="mt-3 font-black leading-tight">A &amp; B</div></a>
			 <div class="x bg-orange-500 y">1998</div>
			 <a href="/manga/2/c" class="mb-2"><div class="font-black">C</div></a>`))
		case strings.HasPrefix(r.URL.Path, "/manga/"):
			w.Write([]byte(`<a class="b" href="/chapters/1-10002000/c-2" title=" Chapter 2">Chapter 2</a>
			 <a class="b" href="/chapters/1-10010000/c-10" title=" Chapter 10">Chapter 10</a>
			 <a class="b" href="/chapters/1-9999/dup" title=" Chapter 2">Chapter 2</a>`))
		default:
			w.Write([]byte(`<img class="js-page" data-src="https://cdn/1.jpeg?t=1"><img class="js-page" data-src="https://cdn/2.jpeg?t=1">`))
		}
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL}
	ctx := context.Background()

	ms, err := c.Search(ctx, "x")
	if err != nil || len(ms) != 2 || ms[0].Title != "A & B" || ms[0].Year != 1998 || ms[1].Year != 0 || ms[0].Source != "Mangapill" {
		t.Fatalf("%+v %v", ms, err)
	}
	chs, _, err := c.Chapters(ctx, ms[0].ID)
	if err != nil || len(chs) != 2 || chs[0].Number != "2" || chs[1].Number != "10" {
		t.Fatalf("%+v %v", chs, err)
	}
	urls, err := c.PageURLs(ctx, chs[0].ID)
	if err != nil || len(urls) != 2 || urls[1] != "https://cdn/2.jpeg?t=1" {
		t.Fatalf("%v %v", urls, err)
	}
}
