package database

import (
	"time"

	"app/internal/config"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// User adalah model tabel users.
type User struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"size:255;uniqueIndex;not null" json:"username"`
	Email     string    `gorm:"size:255;uniqueIndex;not null" json:"email"`
	Password  string    `gorm:"type:text;not null" json:"-"`
	Role      string    `gorm:"size:50;not null;default:member" json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Post adalah model tabel posts.
type Post struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	UserID         uint       `gorm:"not null;index" json:"user_id"`
	User           *User      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Title          string     `gorm:"size:255;not null" json:"title"`
	Slug           string     `gorm:"size:255;uniqueIndex;not null" json:"slug"`
	Category       string     `gorm:"size:100;not null;default:Umum" json:"category"`
	Excerpt        string     `gorm:"type:text" json:"excerpt"`
	Content        string     `gorm:"type:text" json:"content"`
	CoverImagePath string     `gorm:"type:text" json:"cover_image_path"`
	IsPublished    bool       `gorm:"not null;default:false" json:"is_published"`
	PublishedAt    *time.Time `json:"published_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// dialector memilih driver: SQLite in-memory untuk pengujian mandiri,
// PostgreSQL untuk runtime aplikasi.
func dialector(cfg *config.Config) gorm.Dialector {
	if cfg.DBConnection == "sqlite" {
		return sqlite.Open("file:" + cfg.DBDatabase + "?mode=memory&cache=shared")
	}
	return postgres.Open(cfg.DSN())
}

// Connect membuka koneksi basis data sesuai konfigurasi.
func Connect(cfg *config.Config) (*gorm.DB, error) {
	return gorm.Open(dialector(cfg), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
}

// Migrate menjalankan AutoMigrate untuk seluruh model.
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&User{}, &Post{})
}

// Seed membuat akun admin dan member bawaan bila belum ada.
func Seed(db *gorm.DB, cfg *config.Config) error {
	accounts := []struct{ username, email, password, role string }{
		{cfg.AdminUsername, "admin@delcom.org", cfg.AdminPassword, "admin"},
		{"member", "member@delcom.org", "member", "member"},
	}
	for _, a := range accounts {
		if err := seedUser(db, a.username, a.email, a.password, a.role); err != nil {
			return err
		}
	}
	return nil
}

func seedUser(db *gorm.DB, username, email, password, role string) error {
	var count int64
	if err := db.Model(&User{}).Where("username = ?", username).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		if role == "admin" {
			return refreshAdmin(db, username, password)
		}
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return db.Create(&User{Username: username, Email: email, Password: string(hash), Role: role}).Error
}

// refreshAdmin memastikan akun admin bawaan yang sudah ada selalu berperan admin
// dan berpassword sesuai konfigurasi (mis. baris lama pada database yang sudah ada
// dibuat dengan role/password lain).
func refreshAdmin(db *gorm.DB, username, password string) error {
	var user User
	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		return err
	}
	updates := map[string]interface{}{"role": "admin"}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) != nil {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		updates["password"] = string(hash)
	}
	return db.Model(&user).Updates(updates).Error
}

// migrate dapat diganti pada pengujian untuk memaksa kegagalan migrasi.
var migrate = Migrate

// Init menghubungkan database, memigrasikan skema, lalu menjalankan seeder.
func Init(cfg *config.Config) (*gorm.DB, error) {
	db, err := Connect(cfg)
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		return nil, err
	}
	if err := Seed(db, cfg); err != nil {
		return nil, err
	}
	return db, nil
}
