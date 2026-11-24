package auth

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"app/internal/database"
	"app/internal/middleware"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUsernameTaken      = errors.New("username sudah digunakan")
	ErrEmailTaken         = errors.New("email sudah digunakan")
	ErrInvalidCredentials = errors.New("username atau password salah")
)

// Service berisi logika bisnis autentikasi.
type Service struct {
	repo     Repository
	secret   string
	tokenTTL time.Duration
	sign     func(claims jwt.Claims, key []byte) (string, error)
}

// NewService membuat Service dengan token berlaku 24 jam.
func NewService(repo Repository, secret string) *Service {
	return &Service{repo: repo, secret: secret, tokenTTL: 24 * time.Hour, sign: signHS256}
}

func signHS256(claims jwt.Claims, key []byte) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(key)
}

func toUserResponse(u *database.User) UserResponse {
	return UserResponse{ID: u.ID, Username: u.Username, Email: u.Email, Role: u.Role, CreatedAt: u.CreatedAt}
}

// ensureUnique memastikan username dan email belum dipakai pengguna lain.
func (s *Service) ensureUnique(username, email string, selfID uint) error {
	if u, err := s.repo.FindByUsername(username); err == nil {
		if u.ID != selfID {
			return ErrUsernameTaken
		}
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}

	if u, err := s.repo.FindByEmail(email); err == nil {
		if u.ID != selfID {
			return ErrEmailTaken
		}
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	return nil
}

// normalizeRole hanya menerima "admin" atau "member"; selain itu dipaksa "member".
func normalizeRole(role string) string {
	if strings.EqualFold(strings.TrimSpace(role), "admin") {
		return "admin"
	}
	return "member"
}

// register membuat akun member baru dan mengembalikan modelnya.
func (s *Service) register(req RegisterRequest) (*database.User, error) {
	if err := s.ensureUnique(req.Username, req.Email, 0); err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	user := &database.User{Username: req.Username, Email: req.Email, Password: string(hash), Role: normalizeRole(req.Role)}
	if err := s.repo.Create(user); err != nil {
		return nil, err
	}
	return user, nil
}

// Register mendaftarkan akun baru dengan role member.
func (s *Service) Register(req RegisterRequest) (*UserResponse, error) {
	user, err := s.register(req)
	if err != nil {
		return nil, err
	}
	res := toUserResponse(user)
	return &res, nil
}

// Signup mendaftarkan akun baru lalu langsung menerbitkan token JWT.
func (s *Service) Signup(req RegisterRequest) (*LoginResponse, error) {
	user, err := s.register(req)
	if err != nil {
		return nil, err
	}
	return s.issueToken(user)
}

// findByIdentity mencari pengguna lewat username, lalu email.
func (s *Service) findByIdentity(identity string) (*database.User, error) {
	user, err := s.repo.FindByUsername(identity)
	if errors.Is(err, ErrNotFound) {
		return s.repo.FindByEmail(identity)
	}
	return user, err
}

// Login memverifikasi kredensial (username atau email) dan menerbitkan token JWT.
func (s *Service) Login(req LoginRequest) (*LoginResponse, error) {
	user, err := s.findByIdentity(req.Identifier())
	if errors.Is(err, ErrNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)) != nil {
		return nil, ErrInvalidCredentials
	}
	return s.issueToken(user)
}

// issueToken menerbitkan token JWT HS256 untuk pengguna.
func (s *Service) issueToken(user *database.User) (*LoginResponse, error) {
	now := time.Now()
	claims := middleware.Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatUint(uint64(user.ID), 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.tokenTTL)),
		},
	}
	token, err := s.sign(claims, []byte(s.secret))
	if err != nil {
		return nil, err
	}
	return &LoginResponse{Token: token, User: toUserResponse(user)}, nil
}

// GetProfile mengambil profil pengguna berdasarkan ID.
func (s *Service) GetProfile(id uint) (*UserResponse, error) {
	user, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	res := toUserResponse(user)
	return &res, nil
}

// UpdateProfile memperbarui username, email, dan (opsional) password.
func (s *Service) UpdateProfile(id uint, req UpdateProfileRequest) (*UserResponse, error) {
	user, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if req.Username == "" {
		req.Username = user.Username
	}
	if req.Email == "" {
		req.Email = user.Email
	}
	if err := s.ensureUnique(req.Username, req.Email, id); err != nil {
		return nil, err
	}
	user.Username = req.Username
	user.Email = req.Email
	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		user.Password = string(hash)
	}
	if err := s.repo.Update(user); err != nil {
		return nil, err
	}
	res := toUserResponse(user)
	return &res, nil
}
