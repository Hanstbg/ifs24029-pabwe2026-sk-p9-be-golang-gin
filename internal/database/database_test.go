package database

import (
	"errors"
	"strings"
	"testing"

	"app/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func sqliteConfig(name string) *config.Config {
	return &config.Config{
		DBConnection:  "sqlite",
		DBDatabase:    strings.ReplaceAll(name, "/", "_"),
		AdminUsername: "admin",
		AdminPassword: "admin",
	}
}

func closeDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

func TestInitMigratesAndSeedsAccounts(t *testing.T) {
	db, err := Init(sqliteConfig(t.Name()))
	require.NoError(t, err)
	t.Cleanup(func() { closeDB(t, db) })

	var users []User
	require.NoError(t, db.Order("id").Find(&users).Error)
	require.Len(t, users, 2)
	assert.Equal(t, "admin", users[0].Username)
	assert.Equal(t, "admin", users[0].Role)
	assert.Equal(t, "member", users[1].Username)
	assert.Equal(t, "member", users[1].Role)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(users[1].Password), []byte("member")))

	assert.True(t, db.Migrator().HasTable(&Post{}))
}

func TestSeedIsIdempotent(t *testing.T) {
	cfg := sqliteConfig(t.Name())
	db, err := Init(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { closeDB(t, db) })

	require.NoError(t, Seed(db, cfg))

	var count int64
	require.NoError(t, db.Model(&User{}).Count(&count).Error)
	assert.EqualValues(t, 2, count)
}

func TestConnectPostgresFailure(t *testing.T) {
	cfg := &config.Config{
		DBConnection: "pgsql", DBHost: "127.0.0.1", DBPort: "1",
		DBUsername: "u", DBPassword: "p", DBDatabase: "d", DBSSLMode: "disable",
	}

	_, err := Connect(cfg)

	assert.Error(t, err)
}

func TestInitConnectFailure(t *testing.T) {
	cfg := &config.Config{DBConnection: "pgsql", DBHost: "127.0.0.1", DBPort: "1", DBSSLMode: "disable"}

	_, err := Init(cfg)

	assert.Error(t, err)
}

func TestMigrateError(t *testing.T) {
	db, err := Connect(sqliteConfig(t.Name()))
	require.NoError(t, err)
	closeDB(t, db)

	assert.Error(t, Migrate(db))
}

func TestInitMigrateFailure(t *testing.T) {
	original := migrate
	migrate = func(*gorm.DB) error { return errors.New("migrasi gagal") }
	t.Cleanup(func() { migrate = original })

	_, err := Init(sqliteConfig(t.Name()))

	assert.EqualError(t, err, "migrasi gagal")
}

func TestSeedCountError(t *testing.T) {
	cfg := sqliteConfig(t.Name())
	db, err := Connect(cfg)
	require.NoError(t, err)
	require.NoError(t, Migrate(db))
	closeDB(t, db)

	assert.Error(t, Seed(db, cfg))
}

func TestSeedPasswordTooLong(t *testing.T) {
	cfg := sqliteConfig(t.Name())
	cfg.AdminPassword = strings.Repeat("a", 80)

	db, err := Init(cfg)

	assert.Error(t, err)
	assert.Nil(t, db)
}

func TestSeedRestoresExistingAdmin(t *testing.T) {
	cfg := sqliteConfig(t.Name())
	db, err := Init(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { closeDB(t, db) })

	hash, err := bcrypt.GenerateFromPassword([]byte("lain"), bcrypt.MinCost)
	require.NoError(t, err)
	require.NoError(t, db.Model(&User{}).Where("username = ?", "admin").
		Updates(map[string]interface{}{"role": "member", "password": string(hash)}).Error)

	require.NoError(t, Seed(db, cfg))

	var admin User
	require.NoError(t, db.Where("username = ?", "admin").First(&admin).Error)
	assert.Equal(t, "admin", admin.Role)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte("admin")))
}

func TestSeedExistingAdminErrors(t *testing.T) {
	cfg := sqliteConfig(t.Name())
	db, err := Init(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { closeDB(t, db) })

	cfg.AdminPassword = strings.Repeat("b", 80)
	assert.Error(t, Seed(db, cfg))

	assert.Error(t, refreshAdmin(db, "tidak-ada", "x"))
}
