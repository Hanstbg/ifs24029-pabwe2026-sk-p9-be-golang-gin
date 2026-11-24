package auth

import (
	"errors"
	"net/http"

	"app/internal/middleware"
	"app/internal/response"

	"github.com/gin-gonic/gin"
)

// Handler adalah controller endpoint /api/auth/*.
type Handler struct {
	svc *Service
}

// NewHandler membuat Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrUsernameTaken), errors.Is(err, ErrEmailTaken):
		response.Fail(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidCredentials):
		response.Fail(c, http.StatusUnauthorized, err.Error())
	case errors.Is(err, ErrNotFound):
		response.Fail(c, http.StatusNotFound, err.Error())
	default:
		response.Error(c, http.StatusInternalServerError, "Terjadi kesalahan pada server")
	}
}

// Register godoc: POST /api/auth/register
func (h *Handler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "Data registrasi tidak valid: username (min 3), email, dan password (min 6) wajib diisi")
		return
	}
	res, err := h.svc.Signup(req)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "Registrasi berhasil", res)
}

// Login godoc: POST /api/auth/login
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "Username/email dan password wajib diisi")
		return
	}
	if req.Identifier() == "" {
		response.Fail(c, http.StatusBadRequest, "Username/email dan password wajib diisi")
		return
	}
	res, err := h.svc.Login(req)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Login berhasil", res)
}

// Logout godoc: POST /api/auth/logout (JWT stateless, klien menghapus token).
func (h *Handler) Logout(c *gin.Context) {
	response.Success(c, http.StatusOK, "Logout berhasil", nil)
}

// Profile godoc: GET /api/auth/profile
func (h *Handler) Profile(c *gin.Context) {
	user, err := h.svc.GetProfile(middleware.CurrentUser(c).ID)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Profil berhasil diambil", user)
}

// UpdateProfile godoc: PUT /api/auth/profile
func (h *Handler) UpdateProfile(c *gin.Context) {
	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "Data profil tidak valid")
		return
	}
	user, err := h.svc.UpdateProfile(middleware.CurrentUser(c).ID, req)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Profil berhasil diperbarui", user)
}
