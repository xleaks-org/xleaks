package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupCreatesOwnerOnlyLogFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "xleaks.log")

	if err := Setup("info", path); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("log file permissions = %o, want 600", perm)
	}
}

func TestRedactPathPreservesOnlyBasename(t *testing.T) {
	value := RedactPath(filepath.Join("private", "nested", "config.toml"))

	if value.Kind() != slog.KindGroup {
		t.Fatalf("Kind = %v, want %v", value.Kind(), slog.KindGroup)
	}

	attrs := value.Group()
	if len(attrs) != 2 {
		t.Fatalf("group len = %d, want %d", len(attrs), 2)
	}
	if attrs[0].Key != "redacted" || !attrs[0].Value.Bool() {
		t.Fatalf("redacted attr = %v, want true", attrs[0])
	}
	if attrs[1].Key != "base" || attrs[1].Value.String() != "config.toml" {
		t.Fatalf("base attr = %v, want config.toml", attrs[1])
	}
}

func TestRedactAddrHostPortPreservesScopeAndPort(t *testing.T) {
	value := RedactAddr("127.0.0.1:7470")

	if value.Kind() != slog.KindGroup {
		t.Fatalf("Kind = %v, want %v", value.Kind(), slog.KindGroup)
	}

	attrs := value.Group()
	if len(attrs) != 3 {
		t.Fatalf("group len = %d, want %d", len(attrs), 3)
	}
	if attrs[1].Key != "scope" || attrs[1].Value.String() != "loopback" {
		t.Fatalf("scope attr = %v, want loopback", attrs[1])
	}
	if attrs[2].Key != "port" || attrs[2].Value.String() != "7470" {
		t.Fatalf("port attr = %v, want 7470", attrs[2])
	}
}

func TestRedactAddrMultiaddrPreservesProtocolShape(t *testing.T) {
	value := RedactAddr("/ip4/203.0.113.5/tcp/7460/p2p/12D3KooWexample")

	if value.Kind() != slog.KindGroup {
		t.Fatalf("Kind = %v, want %v", value.Kind(), slog.KindGroup)
	}

	attrs := value.Group()
	if len(attrs) != 4 {
		t.Fatalf("group len = %d, want %d", len(attrs), 4)
	}
	if attrs[1].Key != "scope" || attrs[1].Value.String() != "public" {
		t.Fatalf("scope attr = %v, want public", attrs[1])
	}
	if attrs[2].Key != "protocols" {
		t.Fatalf("protocols attr key = %q, want %q", attrs[2].Key, "protocols")
	}
	if attrs[3].Key != "port" || attrs[3].Value.String() != "7460" {
		t.Fatalf("port attr = %v, want 7460", attrs[3])
	}
}

func TestRedactURLPreservesOnlySchemeAndNetworkShape(t *testing.T) {
	value := RedactURL("https://app.example/private/path?q=secret")

	if value.Kind() != slog.KindGroup {
		t.Fatalf("Kind = %v, want %v", value.Kind(), slog.KindGroup)
	}

	attrs := value.Group()
	if len(attrs) != 5 {
		t.Fatalf("group len = %d, want %d", len(attrs), 5)
	}
	if attrs[1].Key != "scheme" || attrs[1].Value.String() != "https" {
		t.Fatalf("scheme attr = %v, want https", attrs[1])
	}
	if attrs[2].Key != "scope" || attrs[2].Value.String() != "hostname" {
		t.Fatalf("scope attr = %v, want hostname", attrs[2])
	}
	if attrs[3].Key != "path_redacted" || !attrs[3].Value.Bool() {
		t.Fatalf("path_redacted attr = %v, want true", attrs[3])
	}
	if attrs[4].Key != "query_redacted" || !attrs[4].Value.Bool() {
		t.Fatalf("query_redacted attr = %v, want true", attrs[4])
	}
}

func TestRedactIdentifierPreservesOnlyFingerprintAndShape(t *testing.T) {
	value := RedactIdentifier("deadbeefcafebabe")

	if value.Kind() != slog.KindGroup {
		t.Fatalf("Kind = %v, want %v", value.Kind(), slog.KindGroup)
	}

	attrs := value.Group()
	if len(attrs) != 4 {
		t.Fatalf("group len = %d, want %d", len(attrs), 4)
	}
	if attrs[1].Key != "fingerprint" || attrs[1].Value.String() == "" {
		t.Fatalf("fingerprint attr = %v, want non-empty fingerprint", attrs[1])
	}
	if attrs[2].Key != "length" || attrs[2].Value.Int64() != int64(len("deadbeefcafebabe")) {
		t.Fatalf("length attr = %v, want %d", attrs[2], len("deadbeefcafebabe"))
	}
	if attrs[3].Key != "format" || attrs[3].Value.String() != "hex" {
		t.Fatalf("format attr = %v, want hex", attrs[3])
	}
}

func TestSetupTightensExistingLogFilePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "xleaks.log")
	if err := os.WriteFile(path, []byte("existing"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := Setup("info", path); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("log file permissions = %o, want 600", perm)
	}
}
