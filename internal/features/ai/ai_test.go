package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errBoom = errors.New("boom")

func boolPtr(b bool) *bool { return &b }

// ---------- SearchClient ----------

func TestSearchSuccess(t *testing.T) {
	var params map[string]string
	var gotUA string
	var hasAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		params = map[string]string{
			"action": q.Get("action"), "list": q.Get("list"), "srsearch": q.Get("srsearch"),
			"srlimit": q.Get("srlimit"), "format": q.Get("format"),
		}
		gotUA = r.Header.Get("User-Agent")
		_, hasAuth = r.Header["Authorization"]
		_, _ = w.Write([]byte(`{"query":{"search":[
			{"title":"Judul A","snippet":"ringkasan <span class=\"searchmatch\">A</span> &quot;dua&quot;\n  baris"},
			{"title":"B","snippet":"b"},
			{"title":"C","snippet":"c"},
			{"title":"D","snippet":"d"},
			{"title":"E","snippet":"e"},
			{"title":"F","snippet":"f"}]}}`))
	}))
	defer srv.Close()

	sources, err := NewSearchClient(srv.URL, srv.Client()).Search(context.Background(), "golang gin")

	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"action": "query", "list": "search", "srsearch": "golang gin", "srlimit": "5", "format": "json",
	}, params)
	assert.NotEmpty(t, gotUA)
	assert.False(t, hasAuth) // kunci OpenAI tidak boleh bocor ke search engine pihak ketiga
	require.Len(t, sources, maxSources)
	assert.Equal(t, "Judul A", sources[0].Title)
	assert.Equal(t, srv.URL+"/wiki/Judul_A", sources[0].URL)
	assert.Equal(t, `ringkasan A "dua" baris`, sources[0].Snippet) // tag HTML dibuang, entitas didekode
}

func TestSearchNoResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"query":{"search":[]}}`))
	}))
	defer srv.Close()

	sources, err := NewSearchClient(srv.URL, srv.Client()).Search(context.Background(), "x")

	require.NoError(t, err)
	assert.Empty(t, sources)
}

func TestSearchErrors(t *testing.T) {
	status := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer status.Close()
	badJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`bukan json`))
	}))
	defer badJSON.Close()
	apiError := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":{"code":"badvalue","info":"parameter tidak valid"}}`))
	}))
	defer apiError.Close()
	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := closed.URL
	closed.Close()

	cases := map[string]*SearchClient{
		"status bukan 200":   NewSearchClient(status.URL, status.Client()),
		"json tidak valid":   NewSearchClient(badJSON.URL, badJSON.Client()),
		"error dari API":     NewSearchClient(apiError.URL, apiError.Client()),
		"endpoint tidak sah": NewSearchClient("://rusak", http.DefaultClient),
		"koneksi ditolak":    NewSearchClient(closedURL, http.DefaultClient),
	}
	for name, client := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := client.Search(context.Background(), "x")
			assert.Error(t, err)
		})
	}
}

// ---------- ChatClient ----------

func chatBody(content string) string {
	b, _ := json.Marshal(map[string]interface{}{
		"choices": []map[string]interface{}{{"message": map[string]string{"role": "assistant", "content": content}}},
	})
	return string(b)
}

func TestChatCompleteSuccess(t *testing.T) {
	var path, auth string
	var payload chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&payload)
		_, _ = w.Write([]byte(chatBody("halo")))
	}))
	defer srv.Close()

	out, err := NewChatClient(srv.URL+"/", "kunci", "model-x", srv.Client()).Complete(context.Background(), "sistem", "pengguna")

	require.NoError(t, err)
	assert.Equal(t, "halo", out)
	assert.Equal(t, "/chat/completions", path)
	assert.Equal(t, "Bearer kunci", auth)
	assert.Equal(t, "model-x", payload.Model)
	require.Len(t, payload.Messages, 2)
	assert.Equal(t, "sistem", payload.Messages[0].Content)
	assert.Equal(t, "pengguna", payload.Messages[1].Content)
}

func TestChatCompleteErrors(t *testing.T) {
	status := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer status.Close()
	badJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`bukan json`))
	}))
	defer badJSON.Close()
	noChoices := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer noChoices.Close()
	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := closed.URL
	closed.Close()

	cases := map[string]*ChatClient{
		"status bukan 200":   NewChatClient(status.URL, "k", "m", status.Client()),
		"json tidak valid":   NewChatClient(badJSON.URL, "k", "m", badJSON.Client()),
		"tanpa choices":      NewChatClient(noChoices.URL, "k", "m", noChoices.Client()),
		"endpoint tidak sah": NewChatClient("://rusak", "k", "m", http.DefaultClient),
		"koneksi ditolak":    NewChatClient(closedURL, "k", "m", http.DefaultClient),
	}
	for name, client := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := client.Complete(context.Background(), "s", "u")
			assert.Error(t, err)
		})
	}
}

// ---------- Service ----------

type stubSearcher struct {
	results []Source
	err     error
	calls   int
	query   string
}

func (s *stubSearcher) Search(_ context.Context, q string) ([]Source, error) {
	s.calls++
	s.query = q
	return s.results, s.err
}

type stubCompleter struct {
	out          string
	err          error
	system, user string
}

func (s *stubCompleter) Complete(_ context.Context, system, user string) (string, error) {
	s.system, s.user = system, user
	return s.out, s.err
}

const goodArticle = `{"title":" Judul AI ","category":"Teknologi","excerpt":" Ringkas ","content":"Isi artikel [1]"}`

var twoSources = []Source{
	{Title: "Sumber Satu", URL: "http://satu", Snippet: "fakta satu"},
	{Title: "Sumber Dua", URL: "http://dua", Snippet: "fakta dua"},
}

func TestGenerateWithWebSearch(t *testing.T) {
	search := &stubSearcher{results: twoSources}
	chat := &stubCompleter{out: goodArticle}

	res, err := NewService(search, chat).Generate(context.Background(), GenerateRequest{Topic: "Golang", Category: "Pendidikan"})

	require.NoError(t, err)
	assert.Equal(t, 1, search.calls)
	assert.Equal(t, "Golang", search.query)
	assert.True(t, res.UsedWebSearch)
	assert.Equal(t, twoSources, res.Sources)
	assert.Equal(t, "Judul AI", res.Title)
	assert.Equal(t, "Ringkas", res.Excerpt)
	assert.Equal(t, "Pendidikan", res.Category) // kategori permintaan menang
	assert.Contains(t, chat.system, "Teknologi, Pendidikan")
	assert.Contains(t, chat.user, "Kategori yang diinginkan: Pendidikan")
	assert.Contains(t, chat.user, "[1] Sumber Satu")
	assert.Contains(t, chat.user, "URL: http://dua")
	assert.Contains(t, chat.user, "## Referensi")
}

func TestGenerateWithoutWebSearch(t *testing.T) {
	search := &stubSearcher{}
	chat := &stubCompleter{out: goodArticle}

	res, err := NewService(search, chat).Generate(context.Background(),
		GenerateRequest{Topic: "Golang", IncludeWebSearch: boolPtr(false)})

	require.NoError(t, err)
	assert.Zero(t, search.calls)
	assert.False(t, res.UsedWebSearch)
	assert.NotNil(t, res.Sources)
	assert.Empty(t, res.Sources)
	assert.Equal(t, "Teknologi", res.Category) // kategori dari AI
	assert.Contains(t, chat.user, "Tidak ada sumber web")
	assert.NotContains(t, chat.user, "Kategori yang diinginkan")
}

func TestGenerateWithEmptySearchResultsFallsBackToNoSources(t *testing.T) {
	chat := &stubCompleter{out: goodArticle}

	res, err := NewService(&stubSearcher{}, chat).Generate(context.Background(), GenerateRequest{Topic: "Golang"})

	require.NoError(t, err)
	assert.True(t, res.UsedWebSearch)
	assert.Contains(t, chat.user, "Tidak ada sumber web")
}

func TestGenerateAcceptsCodeFencedJSON(t *testing.T) {
	chat := &stubCompleter{out: "```json\n" + goodArticle + "\n```"}

	res, err := NewService(&stubSearcher{}, chat).Generate(context.Background(),
		GenerateRequest{Topic: "Golang", IncludeWebSearch: boolPtr(false)})

	require.NoError(t, err)
	assert.Equal(t, "Judul AI", res.Title)
}

func TestGenerateErrors(t *testing.T) {
	req := GenerateRequest{Topic: "Golang"}

	t.Run("search engine gagal", func(t *testing.T) {
		_, err := NewService(&stubSearcher{err: errBoom}, &stubCompleter{out: goodArticle}).Generate(context.Background(), req)
		assert.ErrorIs(t, err, ErrSearch)
	})
	t.Run("openai gagal", func(t *testing.T) {
		_, err := NewService(&stubSearcher{}, &stubCompleter{err: errBoom}).Generate(context.Background(), req)
		assert.ErrorIs(t, err, ErrUpstream)
	})
	for name, out := range map[string]string{
		"tanpa json":      "maaf saya tidak bisa",
		"kurung terbalik": "}{",
		"json rusak":      "{title: bukan json}",
		"judul kosong":    `{"title":"  ","content":"isi"}`,
		"konten kosong":   `{"title":"Judul","content":""}`,
	} {
		t.Run("keluaran AI "+name, func(t *testing.T) {
			_, err := NewService(&stubSearcher{}, &stubCompleter{out: out}).Generate(context.Background(), req)
			assert.ErrorIs(t, err, ErrBadOutput)
		})
	}
}

func TestPickCategory(t *testing.T) {
	assert.Equal(t, "Bisnis", pickCategory("Bisnis", "Teknologi"))
	assert.Equal(t, "Teknologi", pickCategory("Olahraga", "Teknologi"))
	assert.Equal(t, "Teknologi", pickCategory("", "Teknologi"))
	assert.Equal(t, "Umum", pickCategory("Olahraga", "Hiburan"))
}

// ---------- Handler ----------

func generate(t *testing.T, svc *Service, body interface{}) (int, map[string]interface{}) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/generate", NewHandler(svc).Generate)

	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(body))
	req := httptest.NewRequest(http.MethodPost, "/generate", &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return w.Code, out
}

func TestHandlerGenerateSuccess(t *testing.T) {
	svc := NewService(&stubSearcher{results: twoSources}, &stubCompleter{out: goodArticle})

	code, body := generate(t, svc, gin.H{"topic": "Golang untuk pemula"})

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "success", body["status"])
	data := body["data"].(map[string]interface{})
	assert.Equal(t, "Judul AI", data["title"])
	assert.Len(t, data["sources"], 2)
	assert.Equal(t, true, data["used_web_search"])
}

func TestHandlerGenerateRejectsShortTopic(t *testing.T) {
	svc := NewService(&stubSearcher{}, &stubCompleter{out: goodArticle})

	code, body := generate(t, svc, gin.H{"topic": "ab"})

	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "fail", body["status"])
}

func TestHandlerGenerateMapsUpstreamFailuresTo502(t *testing.T) {
	cases := map[string]struct {
		svc  *Service
		want string
	}{
		"search": {NewService(&stubSearcher{err: errBoom}, &stubCompleter{out: goodArticle}), ErrSearch.Error()},
		"openai": {NewService(&stubSearcher{}, &stubCompleter{err: errBoom}), ErrUpstream.Error()},
		"output": {NewService(&stubSearcher{}, &stubCompleter{out: "tanpa json"}), ErrBadOutput.Error()},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			code, body := generate(t, tc.svc, gin.H{"topic": "Golang untuk pemula"})

			assert.Equal(t, http.StatusBadGateway, code)
			assert.Equal(t, "error", body["status"])
			assert.Equal(t, tc.want, body["message"])
		})
	}
}

func TestBuildUserPromptNumbersSources(t *testing.T) {
	prompt := buildUserPrompt(GenerateRequest{Topic: "T"}, twoSources)

	assert.True(t, strings.Contains(prompt, fmt.Sprintf("[%d] %s", 2, "Sumber Dua")))
}
