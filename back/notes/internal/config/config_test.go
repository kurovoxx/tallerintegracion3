package config

import (
	"testing"
	"time"
)

func TestLoadSocialServiceURLDefault(t *testing.T) {
	t.Setenv("SOCIAL_SERVICE_URL", "")
	cfg := Load()
	if cfg.SocialServiceURL != "http://social:8083" {
		t.Fatalf("SocialServiceURL default = %q; want http://social:8083", cfg.SocialServiceURL)
	}
}

func TestLoadSocialServiceURLOverride(t *testing.T) {
	t.Setenv("SOCIAL_SERVICE_URL", "http://127.0.0.1:9099/")
	cfg := Load()
	if cfg.SocialServiceURL != "http://127.0.0.1:9099/" {
		t.Fatalf("SocialServiceURL override = %q", cfg.SocialServiceURL)
	}
}

func TestLoadSocialTimeout(t *testing.T) {
	t.Setenv("SOCIAL_TIMEOUT", "1500ms")
	cfg := Load()
	if cfg.SocialTimeout != 1500*time.Millisecond {
		t.Fatalf("SocialTimeout = %s; want 1.5s", cfg.SocialTimeout)
	}
	t.Setenv("SOCIAL_TIMEOUT", "invalid")
	cfg = Load()
	if cfg.SocialTimeout != 5*time.Second {
		t.Fatalf("invalid SOCIAL_TIMEOUT must fall back to 5s, got %s", cfg.SocialTimeout)
	}
}
