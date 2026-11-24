package auth

import (
	"errors"

	"app/internal/database"

	"gorm.io/gorm"
)

// ErrNotFound dikembalikan bila pengguna tidak ditemukan.
var ErrNotFound = errors.New("pengguna tidak ditemukan")

// Repository mengabstraksi akses data pengguna.
type Repository interface {
	Create(user *database.User) error
	FindByUsername(username string) (*database.User, error)
	FindByEmail(email string) (*database.User, error)
	FindByID(id uint) (*database.User, error)
	Update(user *database.User) error
}

// GormRepository adalah implementasi Repository berbasis GORM.
type GormRepository struct {
	db *gorm.DB
}

// NewRepository membuat GormRepository.
func NewRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{db: db}
}

func (r *GormRepository) Create(user *database.User) error {
	return r.db.Create(user).Error
}

func (r *GormRepository) FindByUsername(username string) (*database.User, error) {
	return r.first("username = ?", username)
}

func (r *GormRepository) FindByEmail(email string) (*database.User, error) {
	return r.first("email = ?", email)
}

func (r *GormRepository) FindByID(id uint) (*database.User, error) {
	return r.first("id = ?", id)
}

func (r *GormRepository) Update(user *database.User) error {
	return r.db.Save(user).Error
}

func (r *GormRepository) first(query string, arg interface{}) (*database.User, error) {
	var user database.User
	err := r.db.Where(query, arg).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}
