package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const secret = "kunci-rahasia-pengujian"

func makeToken(t *testing.T, method jwt.SigningMethod, key string, expiresIn time.Duration, role string) string {
	t.Helper()
	claims := Claims{
		UserID:   7,
		Username: "budi",
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiresIn)),
		},
	}
	token, err := jwt.NewWithClaims(method, claims).SignedString([]byte(key))
	require.NoError(t, err)
	return token
}

func newEngine(handlers ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	handlers = append(handlers, func(c *gin.Context) {
		u := CurrentUser(c)
		c.JSON(http.StatusOK, gin.H{"id": u.ID, "username": u.Username, "role": u.Role})
	})
	r.Any("/x", handlers...)
	return r
}

func do(r http.Handler, method, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/x", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCORSHeadersOnNormalRequest(t *testing.T) {
	w := do(newEngine(CORS()), http.MethodGet, "")

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Authorization")
}

func TestCORSPreflightReturnsNoContent(t *testing.T) {
	w := do(newEngine(CORS()), http.MethodOptions, "")

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "PATCH")
}

func TestAuthRejectsBadHeaders(t *testing.T) {
	r := newEngine(Auth(secret))

	for name, header := range map[string]string{
		"tanpa header": "",
		"bukan bearer": "Basic abc",
		"token kosong": "Bearer ",
		"tanpa spasi":  "Bearer",
		"token acak":   "Bearer bukan.jwt.valid",
		"kunci salah":  "Bearer " + makeToken(t, jwt.SigningMethodHS256, "kunci-lain", time.Hour, "member"),
		"kedaluwarsa":  "Bearer " + makeToken(t, jwt.SigningMethodHS256, secret, -time.Hour, "member"),
		"metode HS512": "Bearer " + makeToken(t, jwt.SigningMethodHS512, secret, time.Hour, "member"),
	} {
		t.Run(name, func(t *testing.T) {
			w := do(r, http.MethodGet, header)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Contains(t, w.Body.String(), `"status":"fail"`)
		})
	}
}

func TestAuthAcceptsValidTokenAndInjectsUser(t *testing.T) {
	r := newEngine(Auth(secret))

	w := do(r, http.MethodGet, "bearer "+makeToken(t, jwt.SigningMethodHS256, secret, time.Hour, "member"))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"id":7,"username":"budi","role":"member"}`, w.Body.String())
}

func TestCurrentUserWithoutAuthIsZeroValue(t *testing.T) {
	w := do(newEngine(), http.MethodGet, "")

	assert.JSONEq(t, `{"id":0,"username":"","role":""}`, w.Body.String())
}

func TestRequireRole(t *testing.T) {
	r := newEngine(Auth(secret), RequireRole("admin", "editor"))

	member := do(r, http.MethodGet, "Bearer "+makeToken(t, jwt.SigningMethodHS256, secret, time.Hour, "member"))
	admin := do(r, http.MethodGet, "Bearer "+makeToken(t, jwt.SigningMethodHS256, secret, time.Hour, "admin"))

	assert.Equal(t, http.StatusForbidden, member.Code)
	assert.Equal(t, http.StatusOK, admin.Code)
}

func TestRateLimiterBlocksAfterLimit(t *testing.T) {
	r := newEngine(NewRateLimiter(2, time.Hour))

	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, "").Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, "").Code)
	blocked := do(r, http.MethodGet, "")

	assert.Equal(t, http.StatusTooManyRequests, blocked.Code)
	assert.Contains(t, blocked.Body.String(), `"status":"fail"`)
}

func TestRateLimiterResetsAfterWindow(t *testing.T) {
	r := newEngine(NewRateLimiter(1, 20*time.Millisecond))

	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, "").Code)
	assert.Equal(t, http.StatusTooManyRequests, do(r, http.MethodGet, "").Code)
	time.Sleep(40 * time.Millisecond)

	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, "").Code)
}

func TestDefaultRateLimiterAllowsNormalTraffic(t *testing.T) {
	assert.Equal(t, http.StatusOK, do(newEngine(RateLimiter()), http.MethodGet, "").Code)
}
