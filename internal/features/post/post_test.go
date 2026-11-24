package post

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

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

func seedAuthor(t *testing.T, db *gorm.DB, username string) uint {
	t.Helper()
	u := &database.User{Username: username, Email: username + "@x.id", Password: "x", Role: "member"}
	require.NoError(t, db.Create(u).Error)
	return u.ID
}

func boolPtr(b bool) *bool { return &b }

// flakyRepo membungkus Repository asli dan dapat dipaksa gagal per metode.
type flakyRepo struct {
	Repository
	errFind, errSlug, errCreate, errUpdate, errDelete    error
	errByUser, errPublished, errAll, errCount, errBySlug error
	errSummaries                                         error
}

func (f flakyRepo) FindByID(id uint) (*database.Post, error) {
	if f.errFind != nil {
		return nil, f.errFind
	}
	return f.Repository.FindByID(id)
}

func (f flakyRepo) SlugExists(slug string, exclude uint) (bool, error) {
	if f.errSlug != nil {
		return false, f.errSlug
	}
	return f.Repository.SlugExists(slug, exclude)
}

func (f flakyRepo) Create(p *database.Post) error {
	if f.errCreate != nil {
		return f.errCreate
	}
	return f.Repository.Create(p)
}

func (f flakyRepo) Update(p *database.Post) error {
	if f.errUpdate != nil {
		return f.errUpdate
	}
	return f.Repository.Update(p)
}

func (f flakyRepo) Delete(p *database.Post) error {
	if f.errDelete != nil {
		return f.errDelete
	}
	return f.Repository.Delete(p)
}

func (f flakyRepo) ListByUser(id uint) ([]database.Post, error) {
	if f.errByUser != nil {
		return nil, f.errByUser
	}
	return f.Repository.ListByUser(id)
}

func (f flakyRepo) ListPublished(fl ListFilter) ([]database.Post, error) {
	if f.errPublished != nil {
		return nil, f.errPublished
	}
	return f.Repository.ListPublished(fl)
}

func (f flakyRepo) ListAll() ([]database.Post, error) {
	if f.errAll != nil {
		return nil, f.errAll
	}
	return f.Repository.ListAll()
}

func (f flakyRepo) CountByUser(id uint) (int64, int64, error) {
	if f.errCount != nil {
		return 0, 0, f.errCount
	}
	return f.Repository.CountByUser(id)
}

func (f flakyRepo) FindPublishedBySlug(slug string) (*database.Post, error) {
	if f.errBySlug != nil {
		return nil, f.errBySlug
	}
	return f.Repository.FindPublishedBySlug(slug)
}

func (f flakyRepo) UserSummaries() ([]UserSummary, error) {
	if f.errSummaries != nil {
		return nil, f.errSummaries
	}
	return f.Repository.UserSummaries()
}

// ---------- fungsi murni ----------

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Hello World!":                   "hello-world",
		"  Belajar   Golang -- Gin  ":    "belajar-golang-gin",
		"Pendidikan di Era AI 2026":      "pendidikan-di-era-ai-2026",
		"!!!":                            "artikel",
		"":                               "artikel",
		strings.Repeat("a", 300):         strings.Repeat("a", maxSlugLength),
		strings.Repeat("a", 199) + " bc": strings.Repeat("a", 199),
	}
	for title, want := range cases {
		assert.Equal(t, want, Slugify(title), "judul: %q", title)
	}
}

func TestReadingTime(t *testing.T) {
	assert.Equal(t, 1, ReadingTime(""))
	assert.Equal(t, 1, ReadingTime(strings.Repeat("kata ", 200)))
	assert.Equal(t, 2, ReadingTime(strings.Repeat("kata ", 201)))
	assert.Equal(t, 3, ReadingTime(strings.Repeat("kata ", 450)))
}

func TestIsValidCategory(t *testing.T) {
	assert.True(t, IsValidCategory("Teknologi"))
	assert.True(t, IsValidCategory("Umum"))
	assert.False(t, IsValidCategory("Olahraga"))
	assert.False(t, IsValidCategory(""))
}

// ---------- service ----------

var fixedNow = time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)

func newService(t *testing.T) (*Service, *gorm.DB, uint) {
	t.Helper()
	db := newTestDB(t)
	uid := seedAuthor(t, db, "penulis")
	svc := NewService(NewRepository(db))
	svc.now = func() time.Time { return fixedNow }
	return svc, db, uid
}

func TestCreate(t *testing.T) {
	svc, _, uid := newService(t)

	draft, err := svc.Create(uid, PostRequest{Title: "  Judul Pertama ", Content: strings.Repeat("kata ", 450)})
	require.NoError(t, err)
	assert.Equal(t, "Judul Pertama", draft.Title)
	assert.Equal(t, "judul-pertama", draft.Slug)
	assert.Equal(t, "Umum", draft.Category)
	assert.False(t, draft.IsPublished)
	assert.Nil(t, draft.PublishedAt)
	assert.Equal(t, 3, draft.ReadingTime)

	dup, err := svc.Create(uid, PostRequest{Title: "Judul Pertama", Content: "isi"})
	require.NoError(t, err)
	assert.Equal(t, "judul-pertama-2", dup.Slug)
	third, err := svc.Create(uid, PostRequest{Title: "Judul Pertama", Content: "isi"})
	require.NoError(t, err)
	assert.Equal(t, "judul-pertama-3", third.Slug)

	published, err := svc.Create(uid, PostRequest{Title: "Terbit", Content: "isi", Category: "Teknologi", IsPublished: boolPtr(true)})
	require.NoError(t, err)
	assert.True(t, published.IsPublished)
	require.NotNil(t, published.PublishedAt)
	assert.True(t, published.PublishedAt.Equal(fixedNow))
}

func TestCreateErrors(t *testing.T) {
	svc, db, uid := newService(t)

	_, err := svc.Create(uid, PostRequest{Title: "x", Content: "y", Category: "Olahraga"})
	assert.ErrorIs(t, err, ErrInvalidCategory)

	flaky := func(f flakyRepo) *Service {
		f.Repository = NewRepository(db)
		return NewService(f)
	}
	_, err = flaky(flakyRepo{errSlug: errDB}).Create(uid, PostRequest{Title: "x", Content: "y"})
	assert.ErrorIs(t, err, errDB)
	_, err = flaky(flakyRepo{errCreate: errDB}).Create(uid, PostRequest{Title: "x", Content: "y"})
	assert.ErrorIs(t, err, errDB)
}

func TestUpdate(t *testing.T) {
	svc, db, uid := newService(t)
	other := seedAuthor(t, db, "lain")
	first, err := svc.Create(uid, PostRequest{Title: "Judul Pertama", Content: "isi awal"})
	require.NoError(t, err)
	_, err = svc.Create(uid, PostRequest{Title: "Judul Kedua", Content: "isi"})
	require.NoError(t, err)

	// judul sama: slug tidak berubah, status publikasi tidak berubah (IsPublished nil)
	same, err := svc.Update(uid, first.ID, UpdatePostRequest{Title: sp("Judul Pertama"), Content: sp("isi baru"), Category: sp("Bisnis")})
	require.NoError(t, err)
	assert.Equal(t, "judul-pertama", same.Slug)
	assert.Equal(t, "isi baru", same.Content)
	assert.Equal(t, "Bisnis", same.Category)
	assert.False(t, same.IsPublished)

	// judul baru: slug dibuat ulang, bentrok dengan artikel lain -> akhiran -2
	renamed, err := svc.Update(uid, first.ID, UpdatePostRequest{Title: sp("Judul Kedua"), Content: sp("isi")})
	require.NoError(t, err)
	assert.Equal(t, "judul-kedua-2", renamed.Slug)

	// terbitkan, lalu terbitkan lagi: published_at tidak berubah
	pub, err := svc.Update(uid, first.ID, UpdatePostRequest{Title: sp("Judul Kedua"), Content: sp("isi"), IsPublished: boolPtr(true)})
	require.NoError(t, err)
	require.NotNil(t, pub.PublishedAt)
	svc.now = func() time.Time { return fixedNow.Add(time.Hour) }
	again, err := svc.Update(uid, first.ID, UpdatePostRequest{Title: sp("Judul Kedua"), Content: sp("isi"), IsPublished: boolPtr(true)})
	require.NoError(t, err)
	assert.True(t, again.PublishedAt.Equal(fixedNow))

	// jadikan draf
	draft, err := svc.Update(uid, first.ID, UpdatePostRequest{Title: sp("Judul Kedua"), Content: sp("isi"), IsPublished: boolPtr(false)})
	require.NoError(t, err)
	assert.False(t, draft.IsPublished)
	assert.Nil(t, draft.PublishedAt)

	// kesalahan
	_, err = svc.Update(other, first.ID, UpdatePostRequest{Title: sp("x"), Content: sp("y")})
	assert.ErrorIs(t, err, ErrForbidden)
	_, err = svc.Update(uid, 9999, UpdatePostRequest{Title: sp("x"), Content: sp("y")})
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = svc.Update(uid, first.ID, UpdatePostRequest{Title: sp("x"), Content: sp("y"), Category: sp("Olahraga")})
	assert.ErrorIs(t, err, ErrInvalidCategory)

	flaky := func(f flakyRepo) *Service {
		f.Repository = NewRepository(db)
		return NewService(f)
	}
	_, err = flaky(flakyRepo{errSlug: errDB}).Update(uid, first.ID, UpdatePostRequest{Title: sp("Judul Berbeda"), Content: sp("y")})
	assert.ErrorIs(t, err, errDB)
	_, err = flaky(flakyRepo{errUpdate: errDB}).Update(uid, first.ID, UpdatePostRequest{Title: sp("Judul Kedua"), Content: sp("y")})
	assert.ErrorIs(t, err, errDB)
}

func TestTogglePublish(t *testing.T) {
	svc, db, uid := newService(t)
	p, err := svc.Create(uid, PostRequest{Title: "Artikel", Content: "isi"})
	require.NoError(t, err)

	on, err := svc.TogglePublish(uid, p.ID)
	require.NoError(t, err)
	assert.True(t, on.IsPublished)
	require.NotNil(t, on.PublishedAt)

	off, err := svc.TogglePublish(uid, p.ID)
	require.NoError(t, err)
	assert.False(t, off.IsPublished)
	assert.Nil(t, off.PublishedAt)

	_, err = svc.TogglePublish(uid+100, p.ID)
	assert.ErrorIs(t, err, ErrForbidden)
	_, err = NewService(flakyRepo{Repository: NewRepository(db), errUpdate: errDB}).TogglePublish(uid, p.ID)
	assert.ErrorIs(t, err, errDB)
}

func TestDeleteGetAndList(t *testing.T) {
	svc, db, uid := newService(t)
	p, err := svc.Create(uid, PostRequest{Title: "Artikel", Content: "isi"})
	require.NoError(t, err)

	got, err := svc.Get(uid, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "penulis", got.Author.Username)

	list, err := svc.List(uid)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	_, err = svc.Get(uid+100, p.ID)
	assert.ErrorIs(t, err, ErrForbidden)
	assert.ErrorIs(t, svc.Delete(uid+100, p.ID), ErrForbidden)
	assert.ErrorIs(t, NewService(flakyRepo{Repository: NewRepository(db), errDelete: errDB}).Delete(uid, p.ID), errDB)
	_, err = NewService(flakyRepo{Repository: NewRepository(db), errByUser: errDB}).List(uid)
	assert.ErrorIs(t, err, errDB)

	require.NoError(t, svc.Delete(uid, p.ID))
	_, err = svc.Get(uid, p.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestStats(t *testing.T) {
	svc, db, uid := newService(t)
	for i, title := range []string{"A", "B", "C"} {
		_, err := svc.Create(uid, PostRequest{Title: title, Content: "isi", IsPublished: boolPtr(i == 0)})
		require.NoError(t, err)
	}

	stats, err := svc.Stats(uid)
	require.NoError(t, err)
	assert.EqualValues(t, 3, stats.TotalArticles)
	assert.EqualValues(t, 3, stats.Total)
	assert.EqualValues(t, 1, stats.TotalSelesai)
	assert.EqualValues(t, 1, stats.TotalTerbit)
	assert.EqualValues(t, 2, stats.BelumSelesai)

	empty, err := svc.Stats(uid + 100)
	require.NoError(t, err)
	assert.EqualValues(t, 0, empty.Total)

	_, err = NewService(flakyRepo{Repository: NewRepository(db), errCount: errDB}).Stats(uid)
	assert.ErrorIs(t, err, errDB)
}

func TestPublicQueries(t *testing.T) {
	svc, db, uid := newService(t)
	mk := func(title, content, category string, publish bool) {
		_, err := svc.Create(uid, PostRequest{Title: title, Content: content, Category: category, Excerpt: "ringkas", IsPublished: boolPtr(publish)})
		require.NoError(t, err)
	}
	mk("Belajar Golang", "Gin framework sangat cepat", "Teknologi", true)
	mk("Resep Nasi Goreng", "Bawang putih dan kecap", "Gaya Hidup", true)
	mk("Draf Rahasia Golang", "belum terbit", "Teknologi", false)

	all, err := svc.ListPublic(ListFilter{})
	require.NoError(t, err)
	assert.Len(t, all, 2)

	byQuery, err := svc.ListPublic(ListFilter{Query: "GOLANG"})
	require.NoError(t, err)
	require.Len(t, byQuery, 1)
	assert.Equal(t, "belajar-golang", byQuery[0].Slug)

	byCategory, err := svc.ListPublic(ListFilter{Category: "Gaya Hidup"})
	require.NoError(t, err)
	assert.Len(t, byCategory, 1)

	none, err := svc.ListPublic(ListFilter{Query: "golang", Category: "Gaya Hidup"})
	require.NoError(t, err)
	assert.Empty(t, none)

	detail, err := svc.GetPublic("belajar-golang")
	require.NoError(t, err)
	assert.Equal(t, "penulis", detail.Author.Username)
	_, err = svc.GetPublic("draf-rahasia-golang")
	assert.ErrorIs(t, err, ErrNotFound)

	assert.Equal(t, Categories, svc.Categories())

	_, err = NewService(flakyRepo{Repository: NewRepository(db), errPublished: errDB}).ListPublic(ListFilter{})
	assert.ErrorIs(t, err, errDB)
	_, err = NewService(flakyRepo{Repository: NewRepository(db), errBySlug: errDB}).GetPublic("x")
	assert.ErrorIs(t, err, errDB)
}

func TestAdminQueries(t *testing.T) {
	svc, db, uid := newService(t)
	_, err := svc.Create(uid, PostRequest{Title: "A", Content: "isi", IsPublished: boolPtr(true)})
	require.NoError(t, err)
	_, err = svc.Create(uid, PostRequest{Title: "B", Content: "isi"})
	require.NoError(t, err)

	users, err := svc.AdminUsers()
	require.NoError(t, err)
	require.Len(t, users, 3) // admin, member (seeder) + penulis
	var summary UserSummary
	for _, u := range users {
		if u.ID == uid {
			summary = u
		}
	}
	assert.EqualValues(t, 2, summary.TotalPosts)
	assert.EqualValues(t, 1, summary.TotalPublished)

	posts, err := svc.AdminPosts()
	require.NoError(t, err)
	assert.Len(t, posts, 2)

	byUser, err := svc.AdminUserPosts(uid)
	require.NoError(t, err)
	assert.Len(t, byUser, 2)

	_, err = NewService(flakyRepo{Repository: NewRepository(db), errAll: errDB}).AdminPosts()
	assert.ErrorIs(t, err, errDB)
	_, err = NewService(flakyRepo{Repository: NewRepository(db), errSummaries: errDB}).AdminUsers()
	assert.ErrorIs(t, err, errDB)
}

func TestRepositoryErrorsOnClosedDatabase(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)
	closeDB(t, db)

	_, err := repo.FindByID(1)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotFound)
	_, err = repo.SlugExists("x", 0)
	assert.Error(t, err)
	_, _, err = repo.CountByUser(1)
	assert.Error(t, err)
}

func TestRepositoryFindNotFound(t *testing.T) {
	repo := NewRepository(newTestDB(t))

	_, err := repo.FindByID(12345)
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = repo.FindPublishedBySlug("tidak-ada")
	assert.ErrorIs(t, err, ErrNotFound)
}

// ---------- handler (HTTP) ----------

func newHandlerRouter(t *testing.T, repo Repository, userID uint) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewHandler(NewService(repo))

	r := gin.New()
	r.GET("/public/categories", h.Categories)
	r.GET("/public/posts", h.PublicList)
	r.GET("/public/posts/:slug", h.PublicDetail)

	fakeAuth := func(c *gin.Context) {
		c.Set(middleware.UserKey, middleware.AuthUser{ID: userID, Username: "penulis", Role: "member"})
	}
	posts := r.Group("/posts", fakeAuth)
	posts.GET("", h.List)
	posts.POST("", h.Create)
	posts.GET("/stats", h.Stats)
	posts.GET("/:id", h.Get)
	posts.PUT("/:id", h.Update)
	posts.PATCH("/:id/toggle-publish", h.TogglePublish)
	posts.DELETE("/:id", h.Delete)

	admin := r.Group("/admin", fakeAuth)
	admin.GET("/users", h.AdminUsers)
	admin.GET("/posts", h.AdminPosts)
	admin.GET("/users/:id/posts", h.AdminUserPosts)
	return r
}

func call(t *testing.T, r http.Handler, method, path string, body interface{}) (int, map[string]interface{}) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return w.Code, out
}

func TestHandlerHappyPath(t *testing.T) {
	db := newTestDB(t)
	uid := seedAuthor(t, db, "penulis")
	r := newHandlerRouter(t, NewRepository(db), uid)

	code, body := call(t, r, http.MethodPost, "/posts",
		gin.H{"title": "Artikel Uji", "content": "isi artikel", "category": "Teknologi", "is_published": true})
	require.Equal(t, http.StatusCreated, code)
	data := body["data"].(map[string]interface{})
	id := int(data["id"].(float64))
	assert.Equal(t, "artikel-uji", data["slug"])

	code, body = call(t, r, http.MethodGet, "/public/categories", nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Len(t, body["data"], len(Categories))

	code, body = call(t, r, http.MethodGet, "/public/posts?q=uji&category=Teknologi", nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Len(t, body["data"], 1)

	code, _ = call(t, r, http.MethodGet, "/public/posts/artikel-uji", nil)
	assert.Equal(t, http.StatusOK, code)
	code, body = call(t, r, http.MethodGet, "/public/posts/tidak-ada", nil)
	assert.Equal(t, http.StatusNotFound, code)
	assert.Equal(t, "fail", body["status"])

	code, body = call(t, r, http.MethodGet, "/posts", nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Len(t, body["data"], 1)

	code, body = call(t, r, http.MethodGet, "/posts/stats", nil)
	assert.Equal(t, http.StatusOK, code)
	assert.EqualValues(t, 1, body["data"].(map[string]interface{})["total_selesai"])

	path := "/posts/" + itoa(id)
	code, _ = call(t, r, http.MethodGet, path, nil)
	assert.Equal(t, http.StatusOK, code)

	code, body = call(t, r, http.MethodPut, path, gin.H{"title": "Artikel Baru", "content": "isi baru"})
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "artikel-baru", body["data"].(map[string]interface{})["slug"])

	code, body = call(t, r, http.MethodPatch, path+"/toggle-publish", nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, false, body["data"].(map[string]interface{})["is_published"])

	code, body = call(t, r, http.MethodGet, "/admin/users", nil)
	assert.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, body["data"])
	code, body = call(t, r, http.MethodGet, "/admin/posts", nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Len(t, body["data"], 1)
	code, body = call(t, r, http.MethodGet, "/admin/users/"+itoa(int(uid))+"/posts", nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Len(t, body["data"], 1)

	code, _ = call(t, r, http.MethodDelete, path, nil)
	assert.Equal(t, http.StatusOK, code)
	code, _ = call(t, r, http.MethodGet, path, nil)
	assert.Equal(t, http.StatusNotFound, code)
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestHandlerClientErrors(t *testing.T) {
	db := newTestDB(t)
	uid := seedAuthor(t, db, "penulis")
	r := newHandlerRouter(t, NewRepository(db), uid)
	good := gin.H{"title": "Judul", "content": "isi"}

	for name, tc := range map[string]struct {
		method, path string
		body         interface{}
		want         int
	}{
		"create tanpa judul":         {http.MethodPost, "/posts", gin.H{"content": "isi"}, http.StatusBadRequest},
		"create kategori salah":      {http.MethodPost, "/posts", gin.H{"title": "J", "content": "i", "category": "Olahraga"}, http.StatusBadRequest},
		"get id tidak valid":         {http.MethodGet, "/posts/abc", nil, http.StatusBadRequest},
		"get tidak ada":              {http.MethodGet, "/posts/999", nil, http.StatusNotFound},
		"update id tidak valid":      {http.MethodPut, "/posts/abc", good, http.StatusBadRequest},
		"update body tidak valid":    {http.MethodPut, "/posts/1", gin.H{"title": 123}, http.StatusBadRequest},
		"update tidak ada":           {http.MethodPut, "/posts/999", good, http.StatusNotFound},
		"toggle id tidak valid":      {http.MethodPatch, "/posts/abc/toggle-publish", nil, http.StatusBadRequest},
		"toggle tidak ada":           {http.MethodPatch, "/posts/999/toggle-publish", nil, http.StatusNotFound},
		"delete id tidak valid":      {http.MethodDelete, "/posts/abc", nil, http.StatusBadRequest},
		"delete tidak ada":           {http.MethodDelete, "/posts/999", nil, http.StatusNotFound},
		"admin posts id tidak valid": {http.MethodGet, "/admin/users/abc/posts", nil, http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			code, body := call(t, r, tc.method, tc.path, tc.body)

			assert.Equal(t, tc.want, code)
			assert.Equal(t, "fail", body["status"])
		})
	}
}

func TestHandlerServerErrorsWhenDatabaseDown(t *testing.T) {
	db := newTestDB(t)
	uid := seedAuthor(t, db, "penulis")
	r := newHandlerRouter(t, NewRepository(db), uid)
	closeDB(t, db)
	good := gin.H{"title": "Judul", "content": "isi"}

	for name, tc := range map[string]struct {
		method, path string
		body         interface{}
	}{
		"public list":      {http.MethodGet, "/public/posts", nil},
		"public detail":    {http.MethodGet, "/public/posts/x", nil},
		"list":             {http.MethodGet, "/posts", nil},
		"stats":            {http.MethodGet, "/posts/stats", nil},
		"create":           {http.MethodPost, "/posts", good},
		"get":              {http.MethodGet, "/posts/1", nil},
		"update":           {http.MethodPut, "/posts/1", good},
		"toggle":           {http.MethodPatch, "/posts/1/toggle-publish", nil},
		"delete":           {http.MethodDelete, "/posts/1", nil},
		"admin users":      {http.MethodGet, "/admin/users", nil},
		"admin posts":      {http.MethodGet, "/admin/posts", nil},
		"admin user posts": {http.MethodGet, "/admin/users/1/posts", nil},
	} {
		t.Run(name, func(t *testing.T) {
			code, body := call(t, r, tc.method, tc.path, tc.body)

			assert.Equal(t, http.StatusInternalServerError, code)
			assert.Equal(t, "error", body["status"])
		})
	}
}


func sp(v string) *string { return &v }

func TestUpdateIsPartial(t *testing.T) {
	svc, _, uid := newService(t)
	p, err := svc.Create(uid, PostRequest{Title: "Asli", Content: "isi asli", Excerpt: "ringkas", Category: "Bisnis"})
	require.NoError(t, err)

	got, err := svc.Update(uid, p.ID, UpdatePostRequest{Title: sp("Baru"), CoverImagePath: sp("/a.png")})
	require.NoError(t, err)
	assert.Equal(t, "Baru", got.Title)
	assert.Equal(t, "isi asli", got.Content)
	assert.Equal(t, "ringkas", got.Excerpt)
	assert.Equal(t, "Bisnis", got.Category)
	assert.Equal(t, "/a.png", got.CoverImagePath)

	_, err = svc.Update(uid, p.ID, UpdatePostRequest{Excerpt: sp("")})
	require.NoError(t, err)
	_, err = svc.Update(uid, p.ID, UpdatePostRequest{Title: sp("  ")})
	assert.ErrorIs(t, err, ErrEmptyField)
	_, err = svc.Update(uid, p.ID, UpdatePostRequest{Content: sp(" ")})
	assert.ErrorIs(t, err, ErrEmptyField)
}

func TestPublicResponseHidesUserID(t *testing.T) {
	svc, _, uid := newService(t)
	_, err := svc.Create(uid, PostRequest{Title: "Publik", Content: "isi", IsPublished: boolPtr(true)})
	require.NoError(t, err)

	list, err := svc.ListPublic(ListFilter{})
	require.NoError(t, err)
	raw, err := json.Marshal(list)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "user_id")
	assert.Contains(t, string(raw), "author")
	assert.Equal(t, "penulis", list[0].AuthorName)
}

func TestHandlerUpdateRejectsEmptyTitle(t *testing.T) {
	db := newTestDB(t)
	uid := seedAuthor(t, db, "penulis")
	r := newHandlerRouter(t, NewRepository(db), uid)

	code, body := call(t, r, http.MethodPost, "/posts", gin.H{"title": "Judul", "content": "isi"})
	require.Equal(t, http.StatusCreated, code)
	id := int(body["data"].(map[string]interface{})["id"].(float64))

	code, body = call(t, r, http.MethodPut, "/posts/"+itoa(id), gin.H{"title": "  "})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "fail", body["status"])
}
