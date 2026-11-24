package post

import (
	"errors"
	"strings"

	"app/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrNotFound dikembalikan bila artikel tidak ditemukan.
var ErrNotFound = errors.New("artikel tidak ditemukan")

// ErrForbidden dikembalikan bila artikel milik penulis lain.
var ErrForbidden = errors.New("anda tidak memiliki akses ke artikel ini")

// Repository mengabstraksi akses data artikel.
type Repository interface {
	Create(post *database.Post) error
	Update(post *database.Post) error
	Delete(post *database.Post) error
	FindByID(id uint) (*database.Post, error)
	FindPublishedBySlug(slug string) (*database.Post, error)
	SlugExists(slug string, excludeID uint) (bool, error)
	ListPublished(filter ListFilter) ([]database.Post, error)
	ListByUser(userID uint) ([]database.Post, error)
	ListAll() ([]database.Post, error)
	CountByUser(userID uint) (total int64, published int64, err error)
	UserSummaries() ([]UserSummary, error)
}

// GormRepository adalah implementasi Repository berbasis GORM.
type GormRepository struct {
	db *gorm.DB
}

// NewRepository membuat GormRepository.
func NewRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{db: db}
}

func (r *GormRepository) Create(post *database.Post) error { return r.db.Create(post).Error }

func (r *GormRepository) Update(post *database.Post) error {
	return r.db.Omit(clause.Associations).Save(post).Error
}

func (r *GormRepository) Delete(post *database.Post) error { return r.db.Delete(post).Error }

func (r *GormRepository) FindByID(id uint) (*database.Post, error) {
	return r.first("id = ?", id, false)
}

func (r *GormRepository) FindPublishedBySlug(slug string) (*database.Post, error) {
	return r.first("slug = ?", slug, true)
}

func (r *GormRepository) first(query string, arg interface{}, publishedOnly bool) (*database.Post, error) {
	q := r.db.Preload("User").Where(query, arg)
	if publishedOnly {
		q = q.Where("is_published = ?", true)
	}
	var post database.Post
	err := q.First(&post).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &post, nil
}

func (r *GormRepository) SlugExists(slug string, excludeID uint) (bool, error) {
	q := r.db.Model(&database.Post{}).Where("slug = ?", slug)
	if excludeID != 0 {
		q = q.Where("id <> ?", excludeID)
	}
	var count int64
	err := q.Count(&count).Error
	return count > 0, err
}

func (r *GormRepository) ListPublished(filter ListFilter) ([]database.Post, error) {
	q := r.db.Preload("User").Where("is_published = ?", true)
	if filter.Category != "" {
		q = q.Where("category = ?", filter.Category)
	}
	if filter.Query != "" {
		like := "%" + strings.ToLower(filter.Query) + "%"
		q = q.Where("LOWER(title) LIKE ? OR LOWER(excerpt) LIKE ? OR LOWER(content) LIKE ?", like, like, like)
	}
	var posts []database.Post
	err := q.Order("published_at DESC, id DESC").Find(&posts).Error
	return posts, err
}

func (r *GormRepository) ListByUser(userID uint) ([]database.Post, error) {
	var posts []database.Post
	err := r.db.Preload("User").Where("user_id = ?", userID).Order("id DESC").Find(&posts).Error
	return posts, err
}

func (r *GormRepository) ListAll() ([]database.Post, error) {
	var posts []database.Post
	err := r.db.Preload("User").Order("id DESC").Find(&posts).Error
	return posts, err
}

func (r *GormRepository) CountByUser(userID uint) (int64, int64, error) {
	var row struct {
		Total     int64
		Published int64
	}
	err := r.db.Model(&database.Post{}).
		Select("COUNT(*) AS total, COALESCE(SUM(CASE WHEN is_published THEN 1 ELSE 0 END), 0) AS published").
		Where("user_id = ?", userID).
		Scan(&row).Error
	return row.Total, row.Published, err
}

func (r *GormRepository) UserSummaries() ([]UserSummary, error) {
	var rows []UserSummary
	err := r.db.Table("users AS u").
		Select("u.id AS id, u.username AS username, u.email AS email, u.role AS role, " +
			"COUNT(p.id) AS total_posts, " +
			"COUNT(p.id) AS total_articles, " +
			"COALESCE(SUM(CASE WHEN p.is_published THEN 1 ELSE 0 END), 0) AS total_published").
		Joins("LEFT JOIN posts AS p ON p.user_id = u.id").
		Group("u.id").
		Order("u.id").
		Scan(&rows).Error
	return rows, err
}
