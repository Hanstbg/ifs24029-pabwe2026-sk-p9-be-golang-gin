package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"app/internal/features/post"
)

// Searcher mengabstraksi Search Engine.
type Searcher interface {
	Search(ctx context.Context, query string) ([]Source, error)
}

// Completer mengabstraksi OpenAI Chat Completions.
type Completer interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// Service menghasilkan draf artikel utuh dengan dukungan penelusuran web.
type Service struct {
	search Searcher
	chat   Completer
}

// NewService membuat Service.
func NewService(search Searcher, chat Completer) *Service {
	return &Service{search: search, chat: chat}
}

type article struct {
	Title    string `json:"title"`
	Category string `json:"category"`
	Excerpt  string `json:"excerpt"`
	Content  string `json:"content"`
}

const systemPrompt = "Anda adalah penulis artikel blog profesional berbahasa Indonesia. " +
	"Balas HANYA dengan satu objek JSON tanpa teks lain, dengan kunci: " +
	"\"title\" (judul menarik), \"category\" (salah satu dari: %s), " +
	"\"excerpt\" (ringkasan pemikat 1-2 kalimat), dan " +
	"\"content\" (isi artikel lengkap dalam format Markdown dengan sub-judul)."

func buildUserPrompt(req GenerateRequest, sources []Source) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Tulis artikel blog tentang topik: %s\n", req.Topic)
	if req.Category != "" {
		fmt.Fprintf(&b, "Kategori yang diinginkan: %s\n", req.Category)
	}
	if len(sources) == 0 {
		b.WriteString("\nTidak ada sumber web. Jangan mengarang kutipan atau tautan referensi.\n")
		return b.String()
	}
	b.WriteString("\nGunakan fakta terkini dari hasil penelusuran berikut dan kutip dengan penanda [1], [2], dst. ")
	b.WriteString("Akhiri artikel dengan bagian \"## Referensi\" berisi daftar tautan sumber yang dipakai.\n\n")
	for i, s := range sources {
		fmt.Fprintf(&b, "[%d] %s\nURL: %s\nRingkasan: %s\n\n", i+1, s.Title, s.URL, s.Snippet)
	}
	return b.String()
}

// parseArticle mengekstrak objek JSON dari balasan AI (toleran terhadap pagar kode ```json).
func parseArticle(raw string) (*article, error) {
	start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return nil, ErrBadOutput
	}
	var a article
	if err := json.Unmarshal([]byte(raw[start:end+1]), &a); err != nil {
		return nil, ErrBadOutput
	}
	if strings.TrimSpace(a.Title) == "" || strings.TrimSpace(a.Content) == "" {
		return nil, ErrBadOutput
	}
	return &a, nil
}

// pickCategory memprioritaskan kategori permintaan, lalu kategori AI, lalu "Umum".
func pickCategory(requested, generated string) string {
	for _, c := range []string{requested, generated} {
		if post.IsValidCategory(c) {
			return c
		}
	}
	return "Umum"
}

// Generate membuat draf artikel; bila IncludeWebSearch aktif, konteks diambil dari Search Engine.
func (s *Service) Generate(ctx context.Context, req GenerateRequest) (*GeneratedPost, error) {
	useSearch := req.IncludeWebSearch == nil || *req.IncludeWebSearch

	sources := []Source{}
	if useSearch {
		found, err := s.search.Search(ctx, req.Topic)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrSearch, err)
		}
		sources = found
	}

	system := fmt.Sprintf(systemPrompt, strings.Join(post.Categories, ", "))
	raw, err := s.chat.Complete(ctx, system, buildUserPrompt(req, sources))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	a, err := parseArticle(raw)
	if err != nil {
		return nil, err
	}

	return &GeneratedPost{
		Title:         strings.TrimSpace(a.Title),
		Category:      pickCategory(req.Category, a.Category),
		Excerpt:       strings.TrimSpace(a.Excerpt),
		Content:       a.Content,
		Sources:       sources,
		UsedWebSearch: useSearch,
	}, nil
}
