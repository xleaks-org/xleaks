package identity

import (
	"encoding/hex"
	"os"
	"testing"
)

func TestWriteActivePubkeyHexUsesOwnerOnlyPermissions(t *testing.T) {
	holder := NewHolder(t.TempDir())

	if err := holder.writeActivePubkeyHex("abc123"); err != nil {
		t.Fatalf("writeActivePubkeyHex() error = %v", err)
	}

	info, err := os.Stat(holder.activeFilePath())
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("active file permissions = %o, want 600", perm)
	}
}

func TestCreateAndSaveCreatesOwnerOnlyIdentityArtifacts(t *testing.T) {
	holder := NewHolder(t.TempDir())

	kp, _, err := holder.CreateAndSave("passphrase")
	if err != nil {
		t.Fatalf("CreateAndSave() error = %v", err)
	}

	keyPath := holder.keyFilePath(hex.EncodeToString(kp.PublicKeyBytes()))
	keyInfo, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("Stat key file error = %v", err)
	}
	if perm := keyInfo.Mode().Perm(); perm != 0o600 {
		t.Fatalf("key file permissions = %o, want 600", perm)
	}

	keysDirInfo, err := os.Stat(holder.keysDir())
	if err != nil {
		t.Fatalf("Stat keys dir error = %v", err)
	}
	if perm := keysDirInfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("keys dir permissions = %o, want 700", perm)
	}

	activeInfo, err := os.Stat(holder.activeFilePath())
	if err != nil {
		t.Fatalf("Stat active file error = %v", err)
	}
	if perm := activeInfo.Mode().Perm(); perm != 0o600 {
		t.Fatalf("active file permissions = %o, want 600", perm)
	}
}
