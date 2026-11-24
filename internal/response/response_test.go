package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serve(t *testing.T, h gin.HandlerFunc) (int, map[string]interface{}) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", h)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return w.Code, body
}

func TestSuccessWithData(t *testing.T) {
	code, body := serve(t, func(c *gin.Context) { Success(c, http.StatusCreated, "dibuat", gin.H{"id": 1}) })

	assert.Equal(t, http.StatusCreated, code)
	assert.Equal(t, "success", body["status"])
	assert.Equal(t, "dibuat", body["message"])
	assert.Equal(t, map[string]interface{}{"id": float64(1)}, body["data"])
}

func TestSuccessWithoutDataOmitsField(t *testing.T) {
	_, body := serve(t, func(c *gin.Context) { Success(c, http.StatusOK, "ok", nil) })

	assert.NotContains(t, body, "data")
}

func TestFail(t *testing.T) {
	code, body := serve(t, func(c *gin.Context) { Fail(c, http.StatusBadRequest, "salah") })

	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "fail", body["status"])
	assert.Equal(t, "salah", body["message"])
}

func TestError(t *testing.T) {
	code, body := serve(t, func(c *gin.Context) { Error(c, http.StatusInternalServerError, "rusak") })

	assert.Equal(t, http.StatusInternalServerError, code)
	assert.Equal(t, "error", body["status"])
}

func TestAbortFailStopsChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reached := false
	r := gin.New()
	r.GET("/", func(c *gin.Context) { AbortFail(c, http.StatusUnauthorized, "tolak") },
		func(c *gin.Context) { reached = true })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"fail"`)
	assert.False(t, reached)
}

func TestAbortErrorStopsChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reached := false
	r := gin.New()
	r.GET("/", func(c *gin.Context) { AbortError(c, http.StatusInternalServerError, "rusak") },
		func(c *gin.Context) { reached = true })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"error"`)
	assert.False(t, reached)
}
