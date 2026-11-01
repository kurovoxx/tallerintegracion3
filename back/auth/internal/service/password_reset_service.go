package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
	"golang.org/x/crypto/bcrypt"
)

type PasswordResetStore interface {
	Issue(context.Context, string, string) (bool, error)
	Complete(context.Context, string, string, string) (bool, error)
}

type PasswordResetMailer interface {
	Ready() bool
	SendResetCode(context.Context, string, string) error
}

type PasswordResetService struct {
	users  UserRepository
	store  PasswordResetStore
	mailer PasswordResetMailer
}

func NewPasswordResetService(users UserRepository, store PasswordResetStore, mailer PasswordResetMailer) *PasswordResetService {
	return &PasswordResetService{users: users, store: store, mailer: mailer}
}

func (s *PasswordResetService) Forgot(ctx context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if !utils.ValidateEmail(email) {
		return NewServiceError(utils.ErrInvalidEmail)
	}
	if s.mailer == nil || !s.mailer.Ready() {
		return &ServiceError{Code: "mail_unavailable", Message: "La recuperación de contraseña no está disponible temporalmente. Inténtalo más tarde."}
	}
	user, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return err
	}
	if user == nil {
		return nil
	}
	n, err := rand.Int(rand.Reader, big.NewInt(100000000))
	if err != nil {
		return err
	}
	code := fmt.Sprintf("%08d", n)
	hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	issued, err := s.store.Issue(ctx, user.ID, string(hash))
	if err != nil {
		return err
	}
	if !issued {
		return nil
	}
	if err := s.mailer.SendResetCode(ctx, user.Email, code); err != nil {
		// Do not expose account existence, SMTP responses, credentials or codes.
		log.Print("password recovery: email delivery failed; check SMTP configuration/connectivity")
	} else {
		log.Print("password recovery: email accepted by SMTP server")
	}
	return nil
}

func (s *PasswordResetService) Reset(ctx context.Context, email, code, password string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if !utils.ValidateEmail(email) {
		return NewServiceError(utils.ErrInvalidEmail)
	}
	code = strings.TrimSpace(code)
	invalid := &ServiceError{Code: "invalid_reset_code", Message: "El código es inválido, expiró o agotó sus intentos. Solicita uno nuevo."}
	if len(code) != 8 {
		return invalid
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return invalid
		}
	}
	if !utils.ValidatePassword(password) {
		return NewServiceError(utils.ErrWeakPassword)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	ok, err := s.store.Complete(ctx, email, code, string(hash))
	if err != nil {
		return err
	}
	if !ok {
		return invalid
	}
	return nil
}
