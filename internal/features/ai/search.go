package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const maxSources = 5

// userAgent wajib diisi: Wikimedia menolak permintaan tanpa User-Agent yang jelas.
const userAgent = "DelcomBlogAI/1.0 (praktikum PABWE IT Del; blog AI generator)"

var (
	htmlTag    = regexp.MustCompile(`<[^>]*>`)
	whitespace = regexp.MustCompile(`\s+`)
)

// SearchClient memanggil MediaWiki Action API (SEARCH_ENGINE_ENDPOINT),
// misalnya https://id.wikipedia.org/w/api.php. Tidak memerlukan API key.
//
// Permintaan: GET {endpoint}?action=query&list=search&srsearch=<kueri>&format=json
// Balasan:    {"query":{"search":[{"title","snippet"(HTML)}]}}
// atau        {"error":{"code","info"}} (dengan status 200).
type SearchClient struct {
	endpoint string
	http     *http.Client
}

// NewSearchClient membuat SearchClient.
func NewSearchClient(endpoint string, hc *http.Client) *SearchClient {
	return &SearchClient{endpoint: endpoint, http: hc}
}

// cleanSnippet membuang tag HTML (mis. <span class="searchmatch">) dan merapikan spasi.
func cleanSnippet(s string) string {
	s = htmlTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(whitespace.ReplaceAllString(s, " "))
}

// Search mengambil hingga 5 sumber teratas untuk kueri.
func (c *SearchClient) Search(ctx context.Context, query string) ([]Source, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("action", "query")
	q.Set("list", "search")
	q.Set("srsearch", query)
	q.Set("srlimit", fmt.Sprint(maxSources))
	q.Set("srprop", "snippet")
	q.Set("utf8", "1")
	q.Set("format", "json")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search engine mengembalikan status %d", res.StatusCode)
	}

	var payload struct {
		Error *struct {
			Code string `json:"code"`
			Info string `json:"info"`
		} `json:"error"`
		Query struct {
			Search []struct {
				Title   string `json:"title"`
				Snippet string `json:"snippet"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.Error != nil {
		return nil, fmt.Errorf("search engine error %s: %s", payload.Error.Code, payload.Error.Info)
	}

	base := url.URL{Scheme: req.URL.Scheme, Host: req.URL.Host}
	sources := make([]Source, 0, len(payload.Query.Search))
	for i, r := range payload.Query.Search {
		if i == maxSources {
			break
		}
		page := base.String() + "/wiki/" + url.PathEscape(strings.ReplaceAll(r.Title, " ", "_"))
		sources = append(sources, Source{Title: r.Title, URL: page, Snippet: cleanSnippet(r.Snippet)})
	}
	return sources, nil
}
