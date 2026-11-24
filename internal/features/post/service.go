package post

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"app/internal/database"
)

// ErrInvalidCategory dikembalikan bila kategori di luar daftar yang diizinkan.
var ErrInvalidCategory = errors.New("kategori tidak valid")

// ErrEmptyField dikembalikan bila judul/konten diubah menjadi kosong.
var ErrEmptyField = errors.New("judul dan konten artikel tidak boleh kosong")

const maxSlugLength = 200

// Service berisi logika bisnis artikel.
type Service struct {
	repo Repository
	now  func() time.Time
}

// NewService membuat Service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// Slugify mengubah judul menjadi slug URL (huruf kecil, angka, dan tanda hubung).
func Slugify(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	slug := strings.TrimRight(b.String(), "-")
	if len(slug) > maxSlugLength {
		slug = strings.TrimRight(slug[:maxSlugLength], "-")
	}
	if slug == "" {
		return "artikel"
	}
	return slug
}

// ReadingTime menghitung estimasi waktu baca dalam menit (kata / 200 wpm, minimal 1).
func ReadingTime(content string) int {
	minutes := (len(strings.Fields(content)) + 199) / 200
	if minutes < 1 {
		return 1
	}
	return minutes
}

func toResponse(p *database.Post) PostResponse {
	res := PostResponse{
		ID:             p.ID,
		UserID:         p.UserID,
		Title:          p.Title,
		Slug:           p.Slug,
		Category:       p.Category,
		Excerpt:        p.Excerpt,
		Content:        p.Content,
		CoverImagePath: p.CoverImagePath,
		IsPublished:    p.IsPublished,
		PublishedAt:    p.PublishedAt,
		ReadingTime:    ReadingTime(p.Content),
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
	if p.User != nil {
		res.Author = &AuthorInfo{ID: p.User.ID, Username: p.User.Username}
	}
	return res
}

func toPublicResponse(p *database.Post) PublicPostResponse {
	res := PublicPostResponse{
		ID:             p.ID,
		Title:          p.Title,
		Slug:           p.Slug,
		Category:       p.Category,
		Excerpt:        p.Excerpt,
		Content:        p.Content,
		CoverImagePath: p.CoverImagePath,
		PublishedAt:    p.PublishedAt,
		ReadingTime:    ReadingTime(p.Content),
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
	if p.User != nil {
		res.Author = &AuthorInfo{ID: p.User.ID, Username: p.User.Username}
		res.AuthorName = p.User.Username
	}
	return res
}

func toResponses(posts []database.Post) []PostResponse {
	out := make([]PostResponse, 0, len(posts))
	for i := range posts {
		out = append(out, toResponse(&posts[i]))
	}
	return out
}

func normalizeCategory(category string) (string, error) {
	category = strings.TrimSpace(category)
	if category == "" {
		return "Umum", nil
	}
	if !IsValidCategory(category) {
		return "", ErrInvalidCategory
	}
	return category, nil
}

// uniqueSlug membuat slug unik dengan akhiran -2, -3, ... bila bentrok.
func (s *Service) uniqueSlug(title string, excludeID uint) (string, error) {
	base := Slugify(title)
	slug := base
	for i := 2; ; i++ {
		exists, err := s.repo.SlugExists(slug, excludeID)
		if err != nil {
			return "", err
		}
		if !exists {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
}

// applyPublish mengatur status publikasi dan published_at. want == nil: tidak berubah.
func (s *Service) applyPublish(p *database.Post, want *bool) {
	if want == nil {
		return
	}
	if !*want {
		p.IsPublished = false
		p.PublishedAt = nil
		return
	}
	if !p.IsPublished {
		now := s.now()
		p.PublishedAt = &now
	}
	p.IsPublished = true
}

// owned mengambil artikel milik penulis; artikel orang lain ditolak (ErrForbidden).
func (s *Service) owned(userID, id uint) (*database.Post, error) {
	p, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if p.UserID != userID {
		return nil, ErrForbidden
	}
	return p, nil
}

// Create membuat artikel baru dengan slug unik.
func (s *Service) Create(userID uint, req PostRequest) (*PostResponse, error) {
	category, err := normalizeCategory(req.Category)
	if err != nil {
		return nil, err
	}
	slug, err := s.uniqueSlug(req.Title, 0)
	if err != nil {
		return nil, err
	}
	p := &database.Post{
		UserID:         userID,
		Title:          strings.TrimSpace(req.Title),
		Slug:           slug,
		Category:       category,
		Excerpt:        req.Excerpt,
		Content:        req.Content,
		CoverImagePath: req.CoverImagePath,
	}
	s.applyPublish(p, req.IsPublished)
	if err := s.repo.Create(p); err != nil {
		return nil, err
	}
	res := toResponse(p)
	return &res, nil
}

// Update memperbarui sebagian/seluruh field artikel milik penulis.
// Slug dibuat ulang bila judul berubah.
func (s *Service) Update(userID, id uint, req UpdatePostRequest) (*PostResponse, error) {
	p, err := s.owned(userID, id)
	if err != nil {
		return nil, err
	}
	if req.Category != nil {
		category, err := normalizeCategory(*req.Category)
		if err != nil {
			return nil, err
		}
		p.Category = category
	}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			return nil, ErrEmptyField
		}
		if title != p.Title {
			slug, err := s.uniqueSlug(title, p.ID)
			if err != nil {
				return nil, err
			}
			p.Slug = slug
		}
		p.Title = title
	}
	if req.Content != nil {
		if strings.TrimSpace(*req.Content) == "" {
			return nil, ErrEmptyField
		}
		p.Content = *req.Content
	}
	if req.Excerpt != nil {
		p.Excerpt = *req.Excerpt
	}
	if req.CoverImagePath != nil {
		p.CoverImagePath = *req.CoverImagePath
	}
	s.applyPublish(p, req.IsPublished)
	if err := s.repo.Update(p); err != nil {
		return nil, err
	}
	res := toResponse(p)
	return &res, nil
}

// TogglePublish membalik status Draf <-> Terbit.
func (s *Service) TogglePublish(userID, id uint) (*PostResponse, error) {
	p, err := s.owned(userID, id)
	if err != nil {
		return nil, err
	}
	want := !p.IsPublished
	s.applyPublish(p, &want)
	if err := s.repo.Update(p); err != nil {
		return nil, err
	}
	res := toResponse(p)
	return &res, nil
}

// Delete menghapus artikel milik penulis.
func (s *Service) Delete(userID, id uint) error {
	p, err := s.owned(userID, id)
	if err != nil {
		return err
	}
	return s.repo.Delete(p)
}

// Get mengambil satu artikel milik penulis.
func (s *Service) Get(userID, id uint) (*PostResponse, error) {
	p, err := s.owned(userID, id)
	if err != nil {
		return nil, err
	}
	res := toResponse(p)
	return &res, nil
}

// List mengambil seluruh artikel milik penulis (terbit dan draf).
func (s *Service) List(userID uint) ([]PostResponse, error) {
	posts, err := s.repo.ListByUser(userID)
	if err != nil {
		return nil, err
	}
	return toResponses(posts), nil
}

// Stats menghitung ringkasan 3 kartu statistik penulis.
func (s *Service) Stats(userID uint) (*Stats, error) {
	total, published, err := s.repo.CountByUser(userID)
	if err != nil {
		return nil, err
	}
	return &Stats{
		TotalArticles: total,
		TotalSelesai:  published,
		BelumSelesai:  total - published,
		Total:         total,
		TotalTerbit:   published,
	}, nil
}

// Categories mengembalikan daftar kategori artikel.
func (s *Service) Categories() []string {
	return Categories
}

// ListPublic mengambil artikel terbit dengan filter kata kunci dan kategori.
func (s *Service) ListPublic(filter ListFilter) ([]PublicPostResponse, error) {
	posts, err := s.repo.ListPublished(filter)
	if err != nil {
		return nil, err
	}
	out := make([]PublicPostResponse, 0, len(posts))
	for i := range posts {
		out = append(out, toPublicResponse(&posts[i]))
	}
	return out, nil
}

// GetPublic mengambil detail artikel terbit berdasarkan slug.
func (s *Service) GetPublic(slug string) (*PublicPostResponse, error) {
	p, err := s.repo.FindPublishedBySlug(slug)
	if err != nil {
		return nil, err
	}
	res := toPublicResponse(p)
	return &res, nil
}

// AdminUsers mengambil ringkasan seluruh pengguna beserta total artikel.
func (s *Service) AdminUsers() ([]UserSummary, error) {
	return s.repo.UserSummaries()
}

// AdminPosts mengambil arsip seluruh artikel dari semua penulis.
func (s *Service) AdminPosts() ([]PostResponse, error) {
	posts, err := s.repo.ListAll()
	if err != nil {
		return nil, err
	}
	return toResponses(posts), nil
}

// AdminUserPosts mengambil seluruh artikel milik pengguna tertentu.
func (s *Service) AdminUserPosts(userID uint) ([]PostResponse, error) {
	return s.List(userID)
}
