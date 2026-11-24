package ai

import "errors"

var (
	// ErrSearch dikembalikan bila Search Engine gagal dihubungi atau membalas error.
	ErrSearch = errors.New("gagal menghubungi mesin pencari")
	// ErrUpstream dikembalikan bila layanan OpenAI gagal dihubungi atau membalas error.
	ErrUpstream = errors.New("gagal menghubungi layanan AI")
	// ErrBadOutput dikembalikan bila keluaran AI tidak berupa artikel JSON yang valid.
	ErrBadOutput = errors.New("respons AI tidak dapat diproses")
)

// GenerateRequest adalah payload POST /api/posts/ai-generate.
// IncludeWebSearch bertipe pointer: nil berarti aktif (default true).
type GenerateRequest struct {
	Topic            string `json:"topic" binding:"required,min=3,max=300"`
	Category         string `json:"category"`
	IncludeWebSearch *bool  `json:"include_web_search"`
}

// Source adalah satu sumber rujukan hasil penelusuran Search Engine.
type Source struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// GeneratedPost adalah draf artikel hasil AI beserta sumber penelusurannya.
type GeneratedPost struct {
	Title         string   `json:"title"`
	Category      string   `json:"category"`
	Excerpt       string   `json:"excerpt"`
	Content       string   `json:"content"`
	Sources       []Source `json:"sources"`
	UsedWebSearch bool     `json:"used_web_search"`
}
