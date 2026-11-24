package ai

import (
	"errors"
	"log"
	"net/http"

	"app/internal/response"

	"github.com/gin-gonic/gin"
)

// Handler adalah controller POST /api/posts/ai-generate.
type Handler struct {
	svc *Service
}

// NewHandler membuat Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Generate menghasilkan draf artikel utuh via Search Engine + OpenAI.
func (h *Handler) Generate(c *gin.Context) {
	var req GenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "Field topic wajib diisi (3-300 karakter)")
		return
	}

	result, err := h.svc.Generate(c.Request.Context(), req)
	if err != nil {
		// Penyebab asli (status upstream, DNS, timeout, dll) hanya dicatat di log server.
		log.Printf("ai-generate gagal: %v", err)
		switch {
		case errors.Is(err, ErrSearch):
			response.Error(c, http.StatusBadGateway, ErrSearch.Error())
		case errors.Is(err, ErrUpstream):
			response.Error(c, http.StatusBadGateway, ErrUpstream.Error())
		default:
			response.Error(c, http.StatusBadGateway, ErrBadOutput.Error())
		}
		return
	}
	response.Success(c, http.StatusOK, "Draf artikel berhasil dibuat", result)
}