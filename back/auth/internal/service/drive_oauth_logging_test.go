package service

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
)

func captureServiceLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

func TestLogging_DriveRefresh_Ok_SinToken(t *testing.T) {
	buf := captureServiceLogs(t)
	exp := time.Now().Add(-1 * time.Hour)
	newExp := time.Now().Add(1 * time.Hour)
	repo := &mockOAuthRepoWithGet{
		conn: &model.OAuthConnection{UserID: "user-1", Provider: model.ProviderGoogleDrive, AccessToken: "old", RefreshToken: stringPtr("refresh123"), ExpiresAt: &exp},
	}
	provider := &mockRefreshProvider{result: &DriveOAuthResult{AccessToken: "TOKEN-SECRETO-XYZ", ExpiresAt: &newExp}}
	svc := NewDriveOAuthServiceWithRefresher(repo, provider)
	token, err := svc.GetValidAccessToken(context.Background(), "user-1")
	if err != nil || token != "TOKEN-SECRETO-XYZ" {
		t.Fatalf("refresh %v %s", err, token)
	}
	out := buf.String()
	if !strings.Contains(out, "drive oauth refresh ok user_id=user-1") {
		t.Fatalf("falta evento ok:\n%s", out)
	}
	if strings.Contains(out, "TOKEN-SECRETO-XYZ") {
		t.Fatalf("el token no debe aparecer en logs:\n%s", out)
	}
}

func TestLogging_DriveRefresh_Revocado_SinToken(t *testing.T) {
	buf := captureServiceLogs(t)
	exp := time.Now().Add(-1 * time.Hour)
	repo := &mockOAuthRepoWithGet{
		conn: &model.OAuthConnection{UserID: "user-1", Provider: model.ProviderGoogleDrive, AccessToken: "old", RefreshToken: stringPtr("refresh123"), ExpiresAt: &exp},
	}
	provider := &mockRefreshProvider{err: ErrDriveConnectionInvalid}
	svc := NewDriveOAuthServiceWithRefresher(repo, provider)
	if _, err := svc.GetValidAccessToken(context.Background(), "user-1"); err == nil {
		t.Fatal("esperado error")
	}
	out := buf.String()
	if !strings.Contains(out, "drive oauth refresh failed user_id=user-1 code=drive_connection_invalid") {
		t.Fatalf("falta evento failed:\n%s", out)
	}
	if strings.Contains(out, "refresh123") {
		t.Fatalf("el refresh token no debe aparecer en logs:\n%s", out)
	}
}
