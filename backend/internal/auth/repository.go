package auth

import (
	"database/sql"
	"errors"
	"time"
)

type User struct {
	ID           uint64
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) FindByEmail(email string) (*User, error) {
	u := &User{}
	err := r.db.QueryRow(
		"SELECT id, email, password_hash, created_at FROM users WHERE email = ?",
		email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *Repository) Create(email, passwordHash string) (*User, error) {
	result, err := r.db.Exec(
		"INSERT INTO users (email, password_hash) VALUES (?, ?)",
		email, passwordHash,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &User{
		ID:           uint64(id),
		Email:        email,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now(),
	}, nil
}
