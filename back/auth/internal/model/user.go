package model

import "time"

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

const (
	RoleStudent = "student"
	RoleTeacher = "teacher"
)

func IsValidRole(r string) bool {
	return r == RoleStudent || r == RoleTeacher
}
