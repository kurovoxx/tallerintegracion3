package config

import "testing"

func TestParseEmailVerificationRequired(t *testing.T) {
	for in, want := range map[string]bool{"": false, "  ": false, "false": false, "0": false, "true": true, "TRUE": true, "1": true, " true ": true} {
		got, err := parseEmailVerificationRequired(in)
		if err != nil || got != want {
			t.Fatalf("%q: got %v %v", in, got, err)
		}
	}
	for _, bad := range []string{"yes", "on", "si", "verdadero", "2"} {
		got, err := parseEmailVerificationRequired(bad)
		if err == nil || got {
			t.Fatalf("%q debe ser un error de configuración, got %v %v", bad, got, err)
		}
	}
}
