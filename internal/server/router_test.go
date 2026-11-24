package server

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"app/internal/config"
	"app/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// upstream meniru Search Engine dan OpenAI Chat Completions.
func upstream(t *testing.T) *httptest.Server {
	t.Helper()
	article, err := json.Marshal(map[string]string{
		"title": "Judul AI", "category": "Teknologi", "excerpt": "Ringkas", "content": "Isi artikel [1]",
	})
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"query":{"search":[{"title":"Sumber","snippet":"fakta <span class=\"searchmatch\">terkini</span>"}]}}`))
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{"message": map[string]string{"role": "assistant", "content": string(article)}}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newApp(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	up := upstream(t)
	cfg := &config.Config{
		DBConnection:         "sqlite",
		DBDatabase:           strings.ReplaceAll(t.Name(), "/", "_"),
		AppKey:               "kunci-pengujian-router",
		AdminUsername:        "admin",
		AdminPassword:        "admin",
		SearchEngineEndpoint: up.URL + "/search",
		OpenAIEndpoint:       up.URL + "/v1",
		OpenAIKey:            "key",
		OpenAIModel:          "model",
	}
	db, err := database.Init(cfg)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return NewRouter(cfg, db), db
}

func call(t *testing.T, r http.Handler, method, path string, body interface{}, token string) (int, map[string]interface{}) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return w.Code, out
}

func login(t *testing.T, r http.Handler, username, password string) string {
	t.Helper()
	code, body := call(t, r, http.MethodPost, "/api/auth/login", gin.H{"username": username, "password": password}, "")
	require.Equal(t, http.StatusOK, code)
	return body["data"].(map[string]interface{})["token"].(string)
}

func TestAuthorWorkflowEndToEnd(t *testing.T) {
	r, _ := newApp(t)

	code, _ := call(t, r, http.MethodPost, "/api/auth/register",
		gin.H{"username": "penulis1", "email": "penulis1@x.id", "password": "rahasia"}, "")
	require.Equal(t, http.StatusCreated, code)
	token := login(t, r, "penulis1", "rahasia")

	code, body := call(t, r, http.MethodPost, "/api/posts",
		gin.H{"title": "Belajar Gin", "content": "isi artikel", "category": "Teknologi", "is_published": true}, token)
	require.Equal(t, http.StatusCreated, code)
	data := body["data"].(map[string]interface{})
	id := int(data["id"].(float64))
	assert.Equal(t, "belajar-gin", data["slug"])

	code, body = call(t, r, http.MethodGet, "/api/public/posts?q=gin", nil, "")
	assert.Equal(t, http.StatusOK, code)
	assert.Len(t, body["data"], 1)
	code, _ = call(t, r, http.MethodGet, "/api/public/posts/belajar-gin", nil, "")
	assert.Equal(t, http.StatusOK, code)
	code, body = call(t, r, http.MethodGet, "/api/public/categories", nil, "")
	assert.Equal(t, http.StatusOK, code)
	assert.Len(t, body["data"], 6)

	code, body = call(t, r, http.MethodGet, "/api/posts/stats", nil, token)
	assert.Equal(t, http.StatusOK, code)
	assert.EqualValues(t, 1, body["data"].(map[string]interface{})["total"])

	path := "/api/posts/" + itoa(id)
	code, _ = call(t, r, http.MethodPut, path, gin.H{"title": "Belajar Gin Lanjut", "content": "isi baru"}, token)
	assert.Equal(t, http.StatusOK, code)
	code, body = call(t, r, http.MethodPatch, path+"/toggle-publish", nil, token)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, false, body["data"].(map[string]interface{})["is_published"])
	code, _ = call(t, r, http.MethodDelete, path, nil, token)
	assert.Equal(t, http.StatusOK, code)

	code, body = call(t, r, http.MethodPost, "/api/posts/ai-generate",
		gin.H{"topic": "Tren Golang 2026", "category": "Teknologi"}, token)
	require.Equal(t, http.StatusOK, code)
	ai := body["data"].(map[string]interface{})
	assert.Equal(t, "Judul AI", ai["title"])
	assert.Len(t, ai["sources"], 1)
	assert.Equal(t, true, ai["used_web_search"])

	code, _ = call(t, r, http.MethodPost, "/api/auth/logout", nil, token)
	assert.Equal(t, http.StatusOK, code)
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestProtectedRoutesRequireToken(t *testing.T) {
	r, _ := newApp(t)

	for _, path := range []string{"/api/posts", "/api/posts/stats", "/api/auth/profile", "/api/admin/users"} {
		code, body := call(t, r, http.MethodGet, path, nil, "")
		assert.Equal(t, http.StatusUnauthorized, code, path)
		assert.Equal(t, "fail", body["status"], path)
	}
}

func TestAdminRoutesAreRoleGuarded(t *testing.T) {
	r, _ := newApp(t)
	member := login(t, r, "member", "member")
	admin := login(t, r, "admin", "admin")

	for _, path := range []string{"/api/admin/users", "/api/admin/posts", "/api/admin/users/1/posts"} {
		code, _ := call(t, r, http.MethodGet, path, nil, member)
		assert.Equal(t, http.StatusForbidden, code, path)

		code, body := call(t, r, http.MethodGet, path, nil, admin)
		assert.Equal(t, http.StatusOK, code, path)
		assert.Equal(t, "success", body["status"], path)
	}
}

func TestSwaggerDocumentation(t *testing.T) {
	r, _ := newApp(t)

	req := httptest.NewRequest(http.MethodGet, "/api.json", nil)
	req.Host = "blog.test:8081"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var spec map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &spec))
	assert.Equal(t, "blog.test:8081", spec["host"])
	assert.Equal(t, []interface{}{"http"}, spec["schemes"])

	req = httptest.NewRequest(http.MethodGet, "/api.json", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Contains(t, w.Body.String(), `"schemes":["https"]`)

	req = httptest.NewRequest(http.MethodGet, "/api.json", nil)
	req.TLS = &tls.ConnectionState{}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Contains(t, w.Body.String(), `"schemes":["https"]`)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "swagger")
}

func TestUnknownRouteReturnsJSON404(t *testing.T) {
	r, _ := newApp(t)

	code, body := call(t, r, http.MethodGet, "/api/tidak-ada", nil, "")

	assert.Equal(t, http.StatusNotFound, code)
	assert.Equal(t, "fail", body["status"])
	assert.Equal(t, "Endpoint not found", body["message"])
}

func TestCORSPreflight(t *testing.T) {
	r, _ := newApp(t)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, httptest.NewRequest(http.MethodOptions, "/api/posts", nil))

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}
