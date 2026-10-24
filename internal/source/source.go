// Package source defines what a manga site must provide to the pipeline.
package source

import "context"

type Manga struct {
	ID     string
	Title  string
	Year   int
	Source string // Name of the Source this came from
}

type Chapter struct {
	ID     string
	Number string
	Title  string
	Volume string
	Pages  int
}

// Source is a manga site that can be searched and downloaded from.
type Source interface {
	Name() string
	Search(ctx context.Context, title string) ([]Manga, error)
	// Chapters returns downloadable chapters in reading order. The int counts
	// chapters that exist but are only linked off-site.
	Chapters(ctx context.Context, mangaID string) ([]Chapter, int, error)
	PageURLs(ctx context.Context, chapterID string) ([]string, error)
	// Referer is sent when fetching page images, or "" if none is needed.
	Referer() string
}
