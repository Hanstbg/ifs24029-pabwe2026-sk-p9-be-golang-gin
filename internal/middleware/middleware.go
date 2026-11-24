package middleware

import (
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"app/internal/response"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// UserKey adalah kunci Gin context tempat data pengguna login disimpan.
const UserKey = "user"

// Claims adalah payload JWT aplikasi.
type Claims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// AuthUser adalah data pengguna yang disuntikkan ke context setelah autentikasi.
type AuthUser struct {
	ID       uint
	Username string
	Role     string
}

// CurrentUser mengambil pengguna login dari context (zero value bila tidak ada).
func CurrentUser(c *gin.Context) AuthUser {
	v, _ := c.Get(UserKey)
	u, _ := v.(AuthUser)
	return u
}

// CORS memberi akses lintas-domain bagi frontend React.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// Auth memverifikasi Bearer Token JWT (HS256) dan menyuntikkan AuthUser.
func Auth(secretKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		parts := strings.SplitN(c.GetHeader("Authorization"), " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			response.AbortFail(c, http.StatusUnauthorized, "Token autentikasi tidak ditemukan")
			return
		}

		claims := &Claims{}
		_, err := jwt.ParseWithClaims(
			strings.TrimSpace(parts[1]),
			claims,
			func(*jwt.Token) (interface{}, error) { return []byte(secretKey), nil },
			jwt.WithValidMethods([]string{"HS256"}),
		)
		if err != nil {
			response.AbortFail(c, http.StatusUnauthorized, "Token tidak valid atau sudah kedaluwarsa")
			return
		}

		c.Set(UserKey, AuthUser{ID: claims.UserID, Username: claims.Username, Role: claims.Role})
		c.Next()
	}
}

// RequireRole hanya mengizinkan pengguna dengan salah satu role yang diberikan.
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := CurrentUser(c)
		for _, role := range roles {
			if user.Role == role {
				c.Next()
				return
			}
		}
		log.Printf("akses ditolak: user=%q role=%q path=%s", user.Username, user.Role, c.Request.URL.Path)
		response.AbortFail(c, http.StatusForbidden, "Anda tidak memiliki akses ke sumber daya ini")
	}
}

// RateLimiter membatasi 300 request per menit per alamat IP.
func RateLimiter() gin.HandlerFunc {
	return NewRateLimiter(300, time.Minute)
}

// NewRateLimiter membuat pembatas request dengan jendela waktu tetap per IP.
func NewRateLimiter(limit int, window time.Duration) gin.HandlerFunc {
	type entry struct {
		count int
		reset time.Time
	}
	var mu sync.Mutex
	hits := map[string]*entry{}

	return func(c *gin.Context) {
		now := time.Now()
		ip := c.ClientIP()

		mu.Lock()
		e, ok := hits[ip]
		if !ok || now.After(e.reset) {
			e = &entry{reset: now.Add(window)}
			hits[ip] = e
		}
		e.count++
		allowed := e.count <= limit
		mu.Unlock()

		if !allowed {
			response.AbortFail(c, http.StatusTooManyRequests, "Terlalu banyak permintaan, coba lagi nanti")
			return
		}
		c.Next()
	}
}
