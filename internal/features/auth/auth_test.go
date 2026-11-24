package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"app/internal/config"
	"app/internal/database"
	"app/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const testSecret = "rahasia-pengujian-auth"

var errDB = errors.New("db down")

// ---------- helpers ----------

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	cfg := &config.Config{
		DBConnection:  "sqlite",
		DBDatabase:    strings.ReplaceAll(t.Name(), "/", "_"),
		AdminUsername: "admin",
		AdminPassword: "admin",
	}
	db, err := database.Init(cfg)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func closeDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

func hashed(t *testing.T, password string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)
	return string(h)
}

type stubRepo struct {
	byUsername func(string) (*database.User, error)
	byEmail    func(string) (*database.User, error)
	byID       func(uint) (*database.User, error)
	create     func(*database.User) error
	update     func(*database.User) error
}

func (s stubRepo) Create(u *database.User) error {
	if s.create != nil {
		return s.create(u)
	}
	return nil
}

func (s stubRepo) Update(u *database.User) error {
	if s.update != nil {
		return s.update(u)
	}
	return nil
}

func (s stubRepo) FindByUsername(n string) (*database.User, error) {
	if s.byUsername != nil {
		return s.byUsername(n)
	}
	return nil, ErrNotFound
}

func (s stubRepo) FindByEmail(e string) (*database.User, error) {
	if s.byEmail != nil {
		return s.byEmail(e)
	}
	return nil, ErrNotFound
}

func (s stubRepo) FindByID(id uint) (*database.User, error) {
	if s.byID != nil {
		return s.byID(id)
	}
	return nil, ErrNotFound
}

func returnUser(u *database.User, err error) func(string) (*database.User, error) {
	return func(string) (*database.User, error) { return u, err }
}

// ---------- service ----------

func TestRegisterSuccessHashesPassword(t *testing.T) {
	var saved *database.User
	svc := NewService(stubRepo{create: func(u *database.User) error { saved = u; return nil }}, testSecret)

	res, err := svc.Register(RegisterRequest{Username: "budi", Email: "budi@x.id", Password: "rahasia"})

	require.NoError(t, err)
	assert.Equal(t, "budi", res.Username)
	assert.Equal(t, "member", res.Role)
	require.NotNil(t, saved)
	assert.NotEqual(t, "rahasia", saved.Password)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(saved.Password), []byte("rahasia")))
}

func TestRegisterErrors(t *testing.T) {
	req := RegisterRequest{Username: "budi", Email: "budi@x.id", Password: "rahasia"}

	cases := map[string]struct {
		repo stubRepo
		req  RegisterRequest
		want error
	}{
		"username dipakai": {
			repo: stubRepo{byUsername: returnUser(&database.User{ID: 5}, nil)}, req: req, want: ErrUsernameTaken,
		},
		"cek username gagal": {
			repo: stubRepo{byUsername: returnUser(nil, errDB)}, req: req, want: errDB,
		},
		"email dipakai": {
			repo: stubRepo{byEmail: returnUser(&database.User{ID: 5}, nil)}, req: req, want: ErrEmailTaken,
		},
		"cek email gagal": {
			repo: stubRepo{byEmail: returnUser(nil, errDB)}, req: req, want: errDB,
		},
		"simpan gagal": {
			repo: stubRepo{create: func(*database.User) error { return errDB }}, req: req, want: errDB,
		},
		"password terlalu panjang": {
			repo: stubRepo{}, req: RegisterRequest{Username: "budi", Email: "budi@x.id", Password: strings.Repeat("a", 80)},
			want: bcrypt.ErrPasswordTooLong,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			res, err := NewService(tc.repo, testSecret).Register(tc.req)

			assert.Nil(t, res)
			assert.ErrorIs(t, err, tc.want)
		})
	}
}

func TestLoginSuccessIssuesValidToken(t *testing.T) {
	user := &database.User{ID: 7, Username: "budi", Email: "budi@x.id", Role: "member", Password: hashed(t, "rahasia")}
	svc := NewService(stubRepo{byUsername: returnUser(user, nil)}, testSecret)

	res, err := svc.Login(LoginRequest{Username: "budi", Password: "rahasia"})

	require.NoError(t, err)
	assert.Equal(t, "budi", res.User.Username)
	claims := &middleware.Claims{}
	parsed, err := jwt.ParseWithClaims(res.Token, claims, func(*jwt.Token) (interface{}, error) {
		return []byte(testSecret), nil
	})
	require.NoError(t, err)
	assert.True(t, parsed.Valid)
	assert.EqualValues(t, 7, claims.UserID)
	assert.Equal(t, "member", claims.Role)
	assert.Equal(t, "7", claims.Subject)
}

func TestLoginErrors(t *testing.T) {
	user := &database.User{ID: 7, Username: "budi", Role: "member", Password: hashed(t, "rahasia")}

	t.Run("pengguna tidak ada", func(t *testing.T) {
		_, err := NewService(stubRepo{}, testSecret).Login(LoginRequest{Username: "x", Password: "y"})
		assert.ErrorIs(t, err, ErrInvalidCredentials)
	})
	t.Run("repo gagal", func(t *testing.T) {
		_, err := NewService(stubRepo{byUsername: returnUser(nil, errDB)}, testSecret).Login(LoginRequest{Username: "x", Password: "y"})
		assert.ErrorIs(t, err, errDB)
	})
	t.Run("password salah", func(t *testing.T) {
		_, err := NewService(stubRepo{byUsername: returnUser(user, nil)}, testSecret).Login(LoginRequest{Username: "budi", Password: "salah"})
		assert.ErrorIs(t, err, ErrInvalidCredentials)
	})
	t.Run("penandatanganan gagal", func(t *testing.T) {
		svc := NewService(stubRepo{byUsername: returnUser(user, nil)}, testSecret)
		svc.sign = func(jwt.Claims, []byte) (string, error) { return "", errDB }

		_, err := svc.Login(LoginRequest{Username: "budi", Password: "rahasia"})

		assert.ErrorIs(t, err, errDB)
	})
}

func TestGetProfile(t *testing.T) {
	svc := NewService(stubRepo{byID: func(id uint) (*database.User, error) {
		return &database.User{ID: id, Username: "budi", CreatedAt: time.Now()}, nil
	}}, testSecret)

	res, err := svc.GetProfile(3)
	require.NoError(t, err)
	assert.EqualValues(t, 3, res.ID)

	_, err = NewService(stubRepo{}, testSecret).GetProfile(3)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestUpdateProfile(t *testing.T) {
	oldHash := hashed(t, "lama-sekali")
	current := func() *database.User {
		return &database.User{ID: 3, Username: "budi", Email: "budi@x.id", Password: oldHash}
	}
	byID := func(uint) (*database.User, error) { return current(), nil }
	req := UpdateProfileRequest{Username: "budi2", Email: "budi2@x.id"}

	t.Run("tanpa password mempertahankan hash lama", func(t *testing.T) {
		var saved *database.User
		svc := NewService(stubRepo{byID: byID, update: func(u *database.User) error { saved = u; return nil }}, testSecret)

		res, err := svc.UpdateProfile(3, req)

		require.NoError(t, err)
		assert.Equal(t, "budi2", res.Username)
		assert.Equal(t, oldHash, saved.Password)
	})
	t.Run("dengan password mengganti hash", func(t *testing.T) {
		var saved *database.User
		svc := NewService(stubRepo{byID: byID, update: func(u *database.User) error { saved = u; return nil }}, testSecret)

		withPass := req
		withPass.Password = "baru-sekali"
		_, err := svc.UpdateProfile(3, withPass)

		require.NoError(t, err)
		assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(saved.Password), []byte("baru-sekali")))
	})
	t.Run("username dan email milik sendiri diizinkan", func(t *testing.T) {
		self := &database.User{ID: 3}
		svc := NewService(stubRepo{byID: byID, byUsername: returnUser(self, nil), byEmail: returnUser(self, nil)}, testSecret)

		_, err := svc.UpdateProfile(3, req)

		assert.NoError(t, err)
	})
	t.Run("pengguna tidak ada", func(t *testing.T) {
		_, err := NewService(stubRepo{}, testSecret).UpdateProfile(3, req)
		assert.ErrorIs(t, err, ErrNotFound)
	})
	t.Run("username dipakai orang lain", func(t *testing.T) {
		svc := NewService(stubRepo{byID: byID, byUsername: returnUser(&database.User{ID: 99}, nil)}, testSecret)
		_, err := svc.UpdateProfile(3, req)
		assert.ErrorIs(t, err, ErrUsernameTaken)
	})
	t.Run("password terlalu panjang", func(t *testing.T) {
		long := req
		long.Password = strings.Repeat("a", 80)
		_, err := NewService(stubRepo{byID: byID}, testSecret).UpdateProfile(3, long)
		assert.ErrorIs(t, err, bcrypt.ErrPasswordTooLong)
	})
	t.Run("simpan gagal", func(t *testing.T) {
		svc := NewService(stubRepo{byID: byID, update: func(*database.User) error { return errDB }}, testSecret)
		_, err := svc.UpdateProfile(3, req)
		assert.ErrorIs(t, err, errDB)
	})
}

// ---------- repository (SQLite in-memory) ----------

func TestGormRepository(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)
	u := &database.User{Username: "andi", Email: "andi@x.id", Password: "hash", Role: "member"}

	require.NoError(t, repo.Create(u))

	byName, err := repo.FindByUsername("andi")
	require.NoError(t, err)
	assert.Equal(t, u.ID, byName.ID)
	byEmail, err := repo.FindByEmail("andi@x.id")
	require.NoError(t, err)
	assert.Equal(t, u.ID, byEmail.ID)
	byID, err := repo.FindByID(u.ID)
	require.NoError(t, err)
	assert.Equal(t, "andi", byID.Username)

	_, err = repo.FindByUsername("tidak-ada")
	assert.ErrorIs(t, err, ErrNotFound)

	u.Username = "andi2"
	require.NoError(t, repo.Update(u))
	updated, err := repo.FindByID(u.ID)
	require.NoError(t, err)
	assert.Equal(t, "andi2", updated.Username)

	closeDB(t, db)
	_, err = repo.FindByID(u.ID)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotFound)
}

// ---------- handler (HTTP) ----------

func newRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)
	h := NewHandler(NewService(NewRepository(db), testSecret))

	r := gin.New()
	r.POST("/register", h.Register)
	r.POST("/login", h.Login)
	authed := r.Group("/", middleware.Auth(testSecret))
	authed.POST("/logout", h.Logout)
	authed.GET("/profile", h.Profile)
	authed.PUT("/profile", h.UpdateProfile)
	return r, db
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
	code, body := call(t, r, http.MethodPost, "/login", gin.H{"username": username, "password": password}, "")
	require.Equal(t, http.StatusOK, code)
	return body["data"].(map[string]interface{})["token"].(string)
}

func tokenFor(t *testing.T, id uint) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, middleware.Claims{
		UserID: id, Username: "hantu", Role: "member",
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	}).SignedString([]byte(testSecret))
	require.NoError(t, err)
	return token
}

func TestRegisterHandler(t *testing.T) {
	r, _ := newRouter(t)

	code, body := call(t, r, http.MethodPost, "/register",
		gin.H{"username": "penulis1", "email": "penulis1@x.id", "password": "rahasia"}, "")
	assert.Equal(t, http.StatusCreated, code)
	assert.Equal(t, "success", body["status"])
	data := body["data"].(map[string]interface{})
	assert.Equal(t, "member", data["user"].(map[string]interface{})["role"])
	assert.Len(t, strings.Split(data["token"].(string), "."), 3)

	code, _ = call(t, r, http.MethodPost, "/register", gin.H{"username": "a"}, "")
	assert.Equal(t, http.StatusBadRequest, code)

	code, body = call(t, r, http.MethodPost, "/register",
		gin.H{"username": "penulis1", "email": "lain@x.id", "password": "rahasia"}, "")
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "fail", body["status"])

	code, _ = call(t, r, http.MethodPost, "/register",
		gin.H{"username": "penulis2", "email": "member@delcom.org", "password": "rahasia"}, "")
	assert.Equal(t, http.StatusBadRequest, code)
}

func TestLoginHandler(t *testing.T) {
	r, _ := newRouter(t)

	token := login(t, r, "member", "member")
	assert.NotEmpty(t, token)

	code, _ := call(t, r, http.MethodPost, "/login", gin.H{"username": "member"}, "")
	assert.Equal(t, http.StatusBadRequest, code)

	code, _ = call(t, r, http.MethodPost, "/login", gin.H{"password": "member"}, "")
	assert.Equal(t, http.StatusBadRequest, code)

	code, body := call(t, r, http.MethodPost, "/login", gin.H{"email": "member@delcom.org", "password": "member"}, "")
	assert.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, body["data"].(map[string]interface{})["token"])

	code, body = call(t, r, http.MethodPost, "/login", gin.H{"username": "member", "password": "salah"}, "")
	assert.Equal(t, http.StatusUnauthorized, code)
	assert.Equal(t, "fail", body["status"])
}

func TestLogoutHandler(t *testing.T) {
	r, _ := newRouter(t)
	token := login(t, r, "member", "member")

	code, body := call(t, r, http.MethodPost, "/logout", nil, token)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "success", body["status"])

	code, _ = call(t, r, http.MethodPost, "/logout", nil, "")
	assert.Equal(t, http.StatusUnauthorized, code)
}

func TestProfileHandlers(t *testing.T) {
	r, _ := newRouter(t)
	token := login(t, r, "member", "member")

	code, body := call(t, r, http.MethodGet, "/profile", nil, token)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "member", body["data"].(map[string]interface{})["username"])

	code, body = call(t, r, http.MethodPut, "/profile",
		gin.H{"username": "member2", "email": "member2@x.id", "password": "baru-sekali"}, token)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "member2", body["data"].(map[string]interface{})["username"])
	assert.NotEmpty(t, login(t, r, "member2", "baru-sekali"))

	code, _ = call(t, r, http.MethodPut, "/profile", gin.H{"username": "x"}, token)
	assert.Equal(t, http.StatusBadRequest, code)

	code, body = call(t, r, http.MethodPut, "/profile", gin.H{"email": "hanya-email@x.id"}, token)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "hanya-email@x.id", body["data"].(map[string]interface{})["email"])
	assert.Equal(t, "member2", body["data"].(map[string]interface{})["username"])

	code, _ = call(t, r, http.MethodPut, "/profile", gin.H{"username": "admin", "email": "m@x.id"}, token)
	assert.Equal(t, http.StatusBadRequest, code)
}

func TestProfileOfDeletedUserIsNotFound(t *testing.T) {
	r, _ := newRouter(t)

	code, body := call(t, r, http.MethodGet, "/profile", nil, tokenFor(t, 9999))

	assert.Equal(t, http.StatusNotFound, code)
	assert.Equal(t, "fail", body["status"])
}

func TestHandlersReturn500WhenDatabaseDown(t *testing.T) {
	r, db := newRouter(t)
	token := login(t, r, "member", "member")
	closeDB(t, db)

	for name, tc := range map[string]struct {
		method, path string
		body         interface{}
		token        string
	}{
		"register": {http.MethodPost, "/register", gin.H{"username": "baru1", "email": "baru1@x.id", "password": "rahasia"}, ""},
		"login":    {http.MethodPost, "/login", gin.H{"username": "member", "password": "member"}, ""},
		"profile":  {http.MethodGet, "/profile", nil, token},
		"update":   {http.MethodPut, "/profile", gin.H{"username": "member", "email": "member@delcom.org"}, token},
	} {
		t.Run(name, func(t *testing.T) {
			code, body := call(t, r, tc.method, tc.path, tc.body, tc.token)

			assert.Equal(t, http.StatusInternalServerError, code)
			assert.Equal(t, "error", body["status"])
		})
	}
}

func TestRegisterRoleIsNormalized(t *testing.T) {
	for role, want := range map[string]string{"admin": "admin", " Admin ": "admin", "member": "member", "": "member", "superuser": "member"} {
		var saved *database.User
		svc := NewService(stubRepo{create: func(u *database.User) error { saved = u; return nil }}, testSecret)

		res, err := svc.Signup(RegisterRequest{Username: "budi", Email: "budi@x.id", Password: "rahasia", Role: role})

		require.NoError(t, err)
		assert.Equal(t, want, saved.Role)
		assert.Equal(t, want, res.User.Role)
	}
}
