package handlers

import (
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/xleaks/xleaks/pkg/identity"
)

// createIdentityRequest is the JSON body for POST /api/identity/create.
type createIdentityRequest struct {
	Passphrase string `json:"passphrase"`
}

// importIdentityRequest is the JSON body for POST /api/identity/import.
type importIdentityRequest struct {
	Mnemonic   string `json:"mnemonic"`
	Passphrase string `json:"passphrase"`
}

// unlockIdentityRequest is the JSON body for POST /api/identity/unlock.
type unlockIdentityRequest struct {
	Passphrase string `json:"passphrase"`
}

// CreateIdentity handles POST /api/identity/create.
func (h *Handler) CreateIdentity(w http.ResponseWriter, r *http.Request) {
	var req createIdentityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Generate a new mnemonic.
	mnemonic, err := identity.GenerateMnemonic()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate mnemonic: "+err.Error())
		return
	}

	// Derive seed from mnemonic.
	seed, err := identity.MnemonicToSeed(mnemonic, "")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to derive seed: "+err.Error())
		return
	}

	// Generate key pair from seed.
	kp, err := identity.KeyPairFromSeed(seed)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate key pair: "+err.Error())
		return
	}

	pubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())

	// Derive the bech32 address.
	address, err := identity.PubKeyToAddress(kp.PublicKeyBytes())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to derive address: "+err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"pubkey":   pubkeyHex,
		"address":  address,
		"mnemonic": mnemonic,
	})
}

// ImportIdentity handles POST /api/identity/import.
func (h *Handler) ImportIdentity(w http.ResponseWriter, r *http.Request) {
	var req importIdentityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Mnemonic == "" {
		respondError(w, http.StatusBadRequest, "mnemonic is required")
		return
	}

	if !identity.ValidateMnemonic(req.Mnemonic) {
		respondError(w, http.StatusBadRequest, "invalid mnemonic")
		return
	}

	// Derive seed from mnemonic.
	seed, err := identity.MnemonicToSeed(req.Mnemonic, "")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to derive seed: "+err.Error())
		return
	}

	// Generate key pair from seed.
	kp, err := identity.KeyPairFromSeed(seed)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate key pair: "+err.Error())
		return
	}

	pubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())

	address, err := identity.PubKeyToAddress(kp.PublicKeyBytes())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to derive address: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"pubkey":  pubkeyHex,
		"address": address,
		"status":  "imported",
	})
}

// UnlockIdentity handles POST /api/identity/unlock.
func (h *Handler) UnlockIdentity(w http.ResponseWriter, r *http.Request) {
	var req unlockIdentityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Passphrase == "" {
		respondError(w, http.StatusBadRequest, "passphrase is required")
		return
	}

	// This is a stub - actual implementation would load the encrypted key
	// from disk and decrypt it with the passphrase.
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "unlocked",
		"pubkey": hex.EncodeToString(h.kp.PublicKeyBytes()),
	})
}

// GetActiveIdentity handles GET /api/identity/active.
func (h *Handler) GetActiveIdentity(w http.ResponseWriter, r *http.Request) {
	pubkeyHex := hex.EncodeToString(h.kp.PublicKeyBytes())

	address, err := identity.PubKeyToAddress(h.kp.PublicKeyBytes())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to derive address: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"pubkey":  pubkeyHex,
		"address": address,
		"active":  true,
	})
}

// LockIdentity handles POST /api/identity/lock.
func (h *Handler) LockIdentity(w http.ResponseWriter, r *http.Request) {
	// Stub - actual implementation would clear the in-memory private key.
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "locked",
	})
}

// ListIdentities handles GET /api/identity/list.
func (h *Handler) ListIdentities(w http.ResponseWriter, r *http.Request) {
	// Return the current identity as the only identity for now.
	pubkeyHex := hex.EncodeToString(h.kp.PublicKeyBytes())

	address, err := identity.PubKeyToAddress(h.kp.PublicKeyBytes())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to derive address: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, []map[string]interface{}{
		{
			"pubkey":  pubkeyHex,
			"address": address,
			"active":  true,
		},
	})
}

// SwitchIdentity handles PUT /api/identity/switch/{pubkey}.
func (h *Handler) SwitchIdentity(w http.ResponseWriter, r *http.Request) {
	pubkey, err := parseHexParam(r, "pubkey")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Stub - actual implementation would switch the active key pair.
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "switched",
		"pubkey": hex.EncodeToString(pubkey),
	})
}

// ExportIdentity handles GET /api/identity/export.
func (h *Handler) ExportIdentity(w http.ResponseWriter, r *http.Request) {
	pubkeyHex := hex.EncodeToString(h.kp.PublicKeyBytes())

	address, err := identity.PubKeyToAddress(h.kp.PublicKeyBytes())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to derive address: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"pubkey":  pubkeyHex,
		"address": address,
	})
}
