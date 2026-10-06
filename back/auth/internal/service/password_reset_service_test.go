package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"golang.org/x/crypto/bcrypt"
)

type resetStoreSpy struct {
	hash, email, code, password string
	issued, completed           bool
	calls                       int
}

func (s *resetStoreSpy) Issue(_ context.Context, _ string, hash string) (bool, error) {
	s.hash = hash
	s.calls++
	return s.issued, nil
}
func (s *resetStoreSpy) Complete(_ context.Context, email, code, password string) (bool, error) {
	s.email, s.code, s.password = email, code, password
	s.calls++
	return s.completed, nil
}

type resetMailerSpy struct {
	ready           bool
	recipient, code string
	err             error
}

func (m *resetMailerSpy) Ready() bool { return m.ready }
func (m *resetMailerSpy) SendResetCode(_ context.Context, recipient, code string) error {
	m.recipient, m.code = recipient, code
	return m.err
}

func TestPasswordRecoveryRequest(t *testing.T) {
	users := newMockUserRepo()
	users.usersByEmail["user@example.com"] = &model.User{ID: "user", Email: "user@example.com"}
	for _, tc := range []struct {
		name, email                                   string
		issued, ready, sendError, wantMail, wantError bool
	}{
		{"registered", " USER@example.com ", true, true, false, true, false},
		{"unknown", "unknown@example.com", true, true, false, false, false},
		{"cooldown", "user@example.com", false, true, false, false, false},
		{"delivery failure stays generic", "user@example.com", true, true, true, true, false},
		{"disabled registered", "user@example.com", true, false, false, false, true},
		{"disabled unknown", "unknown@example.com", true, false, false, false, true},
		{"invalid email", "bad", true, true, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &resetStoreSpy{issued: tc.issued}
			mailer := &resetMailerSpy{ready: tc.ready}
			if tc.sendError {
				mailer.err = errors.New("SMTP failed")
			}
			err := NewPasswordResetService(users, store, mailer).Forgot(context.Background(), tc.email)
			if (err != nil) != tc.wantError {
				t.Fatalf("unexpected error: %v", err)
			}
			if (mailer.code != "") != tc.wantMail {
				t.Fatal("unexpected delivery")
			}
			if tc.wantMail {
				if len(mailer.code) != 8 || mailer.recipient != "user@example.com" {
					t.Fatal("invalid email payload")
				}
				if store.hash == mailer.code || bcrypt.CompareHashAndPassword([]byte(store.hash), []byte(mailer.code)) != nil {
					t.Fatal("code must be hashed at rest")
				}
			}
		})
	}
}

func TestPasswordRecoveryReset(t *testing.T) {
	store := &resetStoreSpy{completed: true}
	svc := NewPasswordResetService(nil, store, nil)
	if err := svc.Reset(context.Background(), " USER@example.com ", " 12345678 ", "NewPassword9"); err != nil {
		t.Fatal(err)
	}
	if store.email != "user@example.com" || store.code != "12345678" {
		t.Fatal("normalization failed")
	}
	if bcrypt.CompareHashAndPassword([]byte(store.password), []byte("NewPassword9")) != nil {
		t.Fatal("password must be bcrypt hashed")
	}
	store.completed = false
	if err := svc.Reset(context.Background(), "user@example.com", "12345678", "NewPassword9"); err == nil {
		t.Fatal("invalid/expired/reused code accepted")
	}
	for _, tc := range []struct{ code, password string }{
		{"1234567", "NewPassword9"}, {"abcdefgh", "NewPassword9"},
		{"12345678", "weak"}, {"12345678", "NoDigitsHere"},
		{"12345678", "Password 9"}, {"12345678", strings.Repeat("a", 73) + "1"},
	} {
		before := store.calls
		if err := svc.Reset(context.Background(), "user@example.com", tc.code, tc.password); err == nil {
			t.Fatal("invalid input accepted")
		}
		if store.calls != before {
			t.Fatal("invalid input reached store")
		}
	}
}
