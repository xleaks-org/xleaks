package main

import (
	"log"

	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/storage"
)

// setupIdentity creates the identity holder. The active key pair remains nil
// until the user creates, imports, or unlocks an identity.
func setupIdentity(dataDir string, db *storage.DB) (*identity.Holder, *identity.KeyPair) {
	idHolder := identity.NewHolder(dataDir)
	idHolder.SetDB(db)

	// Log identity status.
	if idHolder.HasIdentity() {
		log.Println("Identity found. Unlock via API to activate.")
	} else {
		log.Println("No identity found. The UI will guide you through onboarding.")
	}

	return idHolder, nil
}
