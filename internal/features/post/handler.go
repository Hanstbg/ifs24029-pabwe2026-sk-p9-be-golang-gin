package post

import (
	"errors"
	"net/http"
	"strconv"

	"app/internal/middleware"
	"app/internal/response"

	"github.com/gin-gonic/gin"
)

// Handler adalah controller endpoint artikel (publik, penulis, dan admin).
type Handler struct {
	svc *Service
}

// NewHandler membuat Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		response.Fail(c, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrForbidden):
		response.Fail(c, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrInvalidCategory), errors.Is(err, ErrEmptyField):
		response.Fail(c, http.StatusBadRequest, err.Error())
	default:
		response.Error(c, http.StatusInternalServerError, "Terjadi kesalahan pada server")
	}
}

func parseID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "ID tidak valid")
		return 0, false
	}
	return uint(id), true
}

// Categories: GET /api/public/categories
func (h *Handler) Categories(c *gin.Context) {
	response.Success(c, http.StatusOK, "Daftar kategori berhasil diambil", h.svc.Categories())
}

// PublicList: GET /api/public/posts?q=...&category=...
func (h *Handler) PublicList(c *gin.Context) {
	posts, err := h.svc.ListPublic(ListFilter{Query: c.Query("q"), Category: c.Query("category")})
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Daftar artikel berhasil diambil", posts)
}

// PublicDetail: GET /api/public/posts/:slug
func (h *Handler) PublicDetail(c *gin.Context) {
	post, err := h.svc.GetPublic(c.Param("slug"))
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Artikel berhasil diambil", post)
}

// List: GET /api/posts
func (h *Handler) List(c *gin.Context) {
	posts, err := h.svc.List(middleware.CurrentUser(c).ID)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Daftar artikel berhasil diambil", posts)
}

// Stats: GET /api/posts/stats
func (h *Handler) Stats(c *gin.Context) {
	stats, err := h.svc.Stats(middleware.CurrentUser(c).ID)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Statistik artikel berhasil diambil", stats)
}

// Create: POST /api/posts
func (h *Handler) Create(c *gin.Context) {
	var req PostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "Judul dan konten artikel wajib diisi")
		return
	}
	post, err := h.svc.Create(middleware.CurrentUser(c).ID, req)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "Artikel berhasil dibuat", post)
}

// Get: GET /api/posts/:id
func (h *Handler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	post, err := h.svc.Get(middleware.CurrentUser(c).ID, id)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Artikel berhasil diambil", post)
}

// Update: PUT /api/posts/:id
func (h *Handler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req UpdatePostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "Data artikel tidak valid")
		return
	}
	post, err := h.svc.Update(middleware.CurrentUser(c).ID, id, req)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Artikel berhasil diperbarui", post)
}

// TogglePublish: PATCH /api/posts/:id/toggle-publish
func (h *Handler) TogglePublish(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	post, err := h.svc.TogglePublish(middleware.CurrentUser(c).ID, id)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Status publikasi berhasil diubah", post)
}

// Delete: DELETE /api/posts/:id
func (h *Handler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(middleware.CurrentUser(c).ID, id); err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Artikel berhasil dihapus", nil)
}

// AdminUsers: GET /api/admin/users
func (h *Handler) AdminUsers(c *gin.Context) {
	users, err := h.svc.AdminUsers()
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Daftar pengguna berhasil diambil", users)
}

// AdminPosts: GET /api/admin/posts
func (h *Handler) AdminPosts(c *gin.Context) {
	posts, err := h.svc.AdminPosts()
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Arsip artikel berhasil diambil", posts)
}

// AdminUserPosts: GET /api/admin/users/:id/posts
func (h *Handler) AdminUserPosts(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	posts, err := h.svc.AdminUserPosts(id)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Artikel pengguna berhasil diambil", posts)
}
