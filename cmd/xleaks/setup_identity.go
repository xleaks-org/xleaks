package main

import (
	"crypto/ed25519"
	"log"

	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/storage"
)

// setupIdentity creates the identity holder and returns a placeholder key pair.
// The real identity is set when the user unlocks or creates one via the API.
func setupIdentity(dataDir string, db *storage.DB) (*identity.Holder, *identity.KeyPair) {
	idHolder := identity.NewHolder(dataDir)
	idHolder.SetDB(db)

	// Log identity status.
	if idHolder.HasIdentity() {
		log.Println("Identity found. Unlock via API to activate.")
	} else {
		log.Println("No identity found. The UI will guide you through onboarding.")
	}

	// Create a placeholder identity for services that require one at init.
	kp := &identity.KeyPair{
		PrivateKey: ed25519.PrivateKey(make([]byte, ed25519.PrivateKeySize)),
		PublicKey:  ed25519.PublicKey(make([]byte, ed25519.PublicKeySize)),
	}

	return idHolder, kp
}
