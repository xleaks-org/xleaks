package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAPITokenFromEnv(t *testing.T) {
	getenv := func(values map[string]string) func(string) string {
		return func(key string) string {
			return values[key]
		}
	}

	t.Run("direct token", func(t *testing.T) {
		token, err := loadAPITokenFromEnv(getenv(map[string]string{
			apiTokenEnvVar: " secret-token ",
		}))
		if err != nil {
			t.Fatalf("loadAPITokenFromEnv() error = %v", err)
		}
		if token != "secret-token" {
			t.Fatalf("token = %q, want %q", token, "secret-token")
		}
	})

	t.Run("token file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "api.token")
		if err := os.WriteFile(path, []byte("file-token\n"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		token, err := loadAPITokenFromEnv(getenv(map[string]string{
			apiTokenFileEnvVar: path,
		}))
		if err != nil {
			t.Fatalf("loadAPITokenFromEnv() error = %v", err)
		}
		if token != "file-token" {
			t.Fatalf("token = %q, want %q", token, "file-token")
		}
	})

	t.Run("both token sources rejected", func(t *testing.T) {
		_, err := loadAPITokenFromEnv(getenv(map[string]string{
			apiTokenEnvVar:     "one",
			apiTokenFileEnvVar: "/tmp/token",
		}))
		if err == nil {
			t.Fatal("expected error when both token sources are set")
		}
	})
}

func TestValidateAPIExposure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		listenAddr string
		token      string
		wantErr    bool
	}{
		{name: "loopback ipv4 without token", listenAddr: "127.0.0.1:7470"},
		{name: "loopback ipv6 without token", listenAddr: "[::1]:7470"},
		{name: "localhost without token", listenAddr: "localhost:7470"},
		{name: "public bind without token", listenAddr: "0.0.0.0:7470", wantErr: true},
		{name: "wildcard host without token", listenAddr: ":7470", wantErr: true},
		{name: "public bind with token", listenAddr: "0.0.0.0:7470", token: "token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateAPIExposure(tt.listenAddr, tt.token)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateAPIExposure() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
