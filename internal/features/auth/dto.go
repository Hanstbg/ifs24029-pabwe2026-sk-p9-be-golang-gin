package auth

import (
	"strings"
	"time"
)

// RegisterRequest adalah payload pendaftaran akun penulis baru.
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=255"`
	Email    string `json:"email" binding:"required,email,max=255"`
	Password string `json:"password" binding:"required,min=6,max=72"`
	// Role opsional: hanya "admin" atau "member" yang diterima; selain itu menjadi "member".
	Role string `json:"role"`
}

// LoginRequest adalah payload login.
// Identitas boleh dikirim lewat username, email, atau identity (username atau email).
type LoginRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Identity string `json:"identity"`
	Password string `json:"password" binding:"required"`
}

// Identifier mengembalikan identitas login pertama yang terisi.
func (r LoginRequest) Identifier() string {
	for _, v := range []string{r.Username, r.Email, r.Identity} {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// UpdateProfileRequest adalah payload pembaruan profil. Seluruh field opsional;
// field yang kosong tidak diubah.
type UpdateProfileRequest struct {
	Username string `json:"username" binding:"omitempty,min=3,max=255"`
	Email    string `json:"email" binding:"omitempty,email,max=255"`
	Password string `json:"password" binding:"omitempty,min=6,max=72"`
}

// UserResponse adalah representasi publik data pengguna.
type UserResponse struct {
	ID        uint      `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// LoginResponse berisi token JWT dan data pengguna.
type LoginResponse struct {
	Token string       `json:"token"`
	User  UserResponse `json:"user"`
}
