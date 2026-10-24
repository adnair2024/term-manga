// Package mangapill scrapes mangapill.com, which has no API.
package mangapill

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/ashwinnair/term-manga/internal/source"
)

const baseURL = "https://mangapill.com"

// Client scrapes mangapill.com.
type Client struct {
	HTTP    *http.Client
	BaseURL string
}

func New() *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second}, BaseURL: baseURL}
}

// Name implements source.Source.
func (c *Client) Name() string { return "Mangapill" }

// Referer implements source.Source; the image CDN returns 403 without it.
func (c *Client) Referer() string { return baseURL + "/" }

var (
	searchHit = regexp.MustCompile(`href="(/manga/\d+/[^"]+)" class="mb-2">\s*<div class="[^"]*font-black[^"]*">([^<]+)</div>`)
	yearTag   = regexp.MustCompile(`bg-orange-500[^>]*>(\d{4})<`)
	chapterA  = regexp.MustCompile(`href="(/chapters/[^"]+)"[^>]*>\s*([^<]*?)\s*</a>`)
	chapterNo = regexp.MustCompile(`(?i)chapter\s+(\d+(?:\.\d+)?)`)
	pageImg   = regexp.MustCompile(`<img[^>]*js-page[^>]*data-src="([^"]+)"`)
)

func (c *Client) page(ctx context.Context, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) Firefox/130.0")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("mangapill %s: %s", path, resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

// Search returns series matching title.
func (c *Client) Search(ctx context.Context, title string) ([]source.Manga, error) {
	doc, err := c.page(ctx, "/search?q="+url.QueryEscape(title))
	if err != nil {
		return nil, err
	}
	hits := searchHit.FindAllStringSubmatchIndex(doc, -1)
	var out []source.Manga
	for i, h := range hits {
		m := source.Manga{ID: doc[h[2]:h[3]], Title: html.UnescapeString(doc[h[4]:h[5]]), Source: c.Name()}
		end := len(doc)
		if i+1 < len(hits) {
			end = hits[i+1][0]
		}
		if y := yearTag.FindStringSubmatch(doc[h[1]:end]); y != nil {
			m.Year, _ = strconv.Atoi(y[1])
		}
		out = append(out, m)
	}
	return out, nil
}

// Chapters returns all chapters named "Chapter N", sorted by number, with
// duplicate numbers collapsed. Mangapill has no volume data.
func (c *Client) Chapters(ctx context.Context, mangaID string) ([]source.Chapter, int, error) {
	doc, err := c.page(ctx, mangaID)
	if err != nil {
		return nil, 0, err
	}
	seen := map[string]bool{}
	keys := map[string]float64{}
	var out []source.Chapter
	for _, m := range chapterA.FindAllStringSubmatch(doc, -1) {
		n := chapterNo.FindStringSubmatch(html.UnescapeString(m[2]))
		if n == nil || seen[n[1]] {
			continue
		}
		seen[n[1]] = true
		out = append(out, source.Chapter{ID: m[1], Number: n[1]})
		keys[m[1]], _ = strconv.ParseFloat(n[1], 64)
	}
	sort.SliceStable(out, func(i, j int) bool { return keys[out[i].ID] < keys[out[j].ID] })
	return out, 0, nil
}

// PageURLs returns the image URLs for a chapter.
func (c *Client) PageURLs(ctx context.Context, chapterID string) ([]string, error) {
	doc, err := c.page(ctx, chapterID)
	if err != nil {
		return nil, err
	}
	var urls []string
	for _, m := range pageImg.FindAllStringSubmatch(doc, -1) {
		urls = append(urls, html.UnescapeString(m[1]))
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("mangapill: no pages found for %s", chapterID)
	}
	return urls, nil
}
