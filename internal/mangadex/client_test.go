package mangadex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChaptersDedupesSortsAndSkipsExternal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"total":4,"data":[
		 {"id":"b","attributes":{"chapter":"10","pages":5}},
		 {"id":"a","attributes":{"chapter":"2","pages":5}},
		 {"id":"dup","attributes":{"chapter":"2","pages":5}},
		 {"id":"ext","attributes":{"chapter":"3","pages":0,"externalUrl":"http://x"}}]}`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL}
	got, ext, err := c.Chapters(context.Background(), "m")
	if err != nil {
		t.Fatal(err)
	}
	if ext != 1 || len(got) != 2 || got[0].Number != "2" || got[1].Number != "10" {
		t.Fatalf("got %+v", got)
	}
}

func TestPageURLs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"baseUrl":"https://cdn","chapter":{"hash":"h","data":["1.png","2.png"]}}`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL}
	got, err := c.PageURLs(context.Background(), "x")
	if err != nil || strings.Join(got, ",") != "https://cdn/data/h/1.png,https://cdn/data/h/2.png" {
		t.Fatalf("%v %v", got, err)
	}
}
