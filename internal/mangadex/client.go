// Package mangadex is a minimal client for the MangaDex REST API.
package mangadex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

const baseURL = "https://api.mangadex.org"

// Client talks to api.mangadex.org.
type Client struct {
	HTTP    *http.Client
	BaseURL string
}

func New() *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second}, BaseURL: baseURL}
}

type Manga struct {
	ID    string
	Title string
	Year  int
}

type Chapter struct {
	ID      string
	Number  string
	Title   string
	Volume  string
	Pages   int
	sortKey float64
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "mangapipeline/0.1")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mangadex %s: %s", path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Search returns manga matching title, English-translated, safe and suggestive only.
func (c *Client) Search(ctx context.Context, title string) ([]Manga, error) {
	q := url.Values{}
	q.Set("title", title)
	q.Set("limit", "25")
	q.Set("order[relevance]", "desc")
	q.Add("contentRating[]", "safe")
	q.Add("contentRating[]", "suggestive")
	q.Add("availableTranslatedLanguage[]", "en")

	var res struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				Title map[string]string   `json:"title"`
				AltT  []map[string]string `json:"altTitles"`
				Year  *int                `json:"year"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := c.get(ctx, "/manga", q, &res); err != nil {
		return nil, err
	}
	out := make([]Manga, 0, len(res.Data))
	for _, d := range res.Data {
		t := d.Attributes.Title["en"]
		if t == "" {
			for _, alt := range d.Attributes.AltT {
				if v := alt["en"]; v != "" {
					t = v
					break
				}
			}
		}
		if t == "" {
			for _, v := range d.Attributes.Title {
				t = v
				break
			}
		}
		m := Manga{ID: d.ID, Title: t}
		if d.Attributes.Year != nil {
			m.Year = *d.Attributes.Year
		}
		out = append(out, m)
	}
	return out, nil
}

// Chapters returns all English chapters for a manga, sorted by chapter number,
// with duplicate chapter numbers (multiple scanlation groups) collapsed.
//
// The second return value counts chapters that exist but are only linked
// off-site (e.g. official publisher readers) and so cannot be downloaded.
func (c *Client) Chapters(ctx context.Context, mangaID string) ([]Chapter, int, error) {
	const pageSize = 100
	seen := map[string]bool{}
	var out []Chapter
	external := 0
	for offset := 0; ; offset += pageSize {
		q := url.Values{}
		q.Set("limit", strconv.Itoa(pageSize))
		q.Set("offset", strconv.Itoa(offset))
		q.Set("order[chapter]", "asc")
		q.Add("translatedLanguage[]", "en")
		q.Add("contentRating[]", "safe")
		q.Add("contentRating[]", "suggestive")

		var res struct {
			Total int `json:"total"`
			Data  []struct {
				ID         string `json:"id"`
				Attributes struct {
					Chapter *string `json:"chapter"`
					Volume  *string `json:"volume"`
					Title   *string `json:"title"`
					Pages   int     `json:"pages"`
					// External chapters link off-site and have no hosted pages.
					ExternalURL *string `json:"externalUrl"`
				} `json:"attributes"`
			} `json:"data"`
		}
		if err := c.get(ctx, "/manga/"+mangaID+"/feed", q, &res); err != nil {
			return nil, 0, err
		}
		for _, d := range res.Data {
			a := d.Attributes
			if a.ExternalURL != nil {
				external++
				continue
			}
			if a.Pages == 0 {
				continue
			}
			ch := Chapter{ID: d.ID, Pages: a.Pages}
			if a.Chapter != nil {
				ch.Number = *a.Chapter
			}
			if ch.Number == "" || seen[ch.Number] {
				continue // oneshots and duplicate uploads
			}
			seen[ch.Number] = true
			if a.Volume != nil {
				ch.Volume = *a.Volume
			}
			if a.Title != nil {
				ch.Title = *a.Title
			}
			ch.sortKey, _ = strconv.ParseFloat(ch.Number, 64)
			out = append(out, ch)
		}
		if offset+pageSize >= res.Total {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].sortKey < out[j].sortKey })
	return out, external, nil
}

// PageURLs returns the image URLs for a chapter via the at-home server.
func (c *Client) PageURLs(ctx context.Context, chapterID string) ([]string, error) {
	var res struct {
		BaseURL string `json:"baseUrl"`
		Chapter struct {
			Hash string   `json:"hash"`
			Data []string `json:"data"`
		} `json:"chapter"`
	}
	if err := c.get(ctx, "/at-home/server/"+chapterID, url.Values{}, &res); err != nil {
		return nil, err
	}
	urls := make([]string, len(res.Chapter.Data))
	for i, f := range res.Chapter.Data {
		urls[i] = fmt.Sprintf("%s/data/%s/%s", res.BaseURL, res.Chapter.Hash, f)
	}
	return urls, nil
}
