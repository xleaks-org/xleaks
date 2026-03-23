package handlers

import (
	"encoding/hex"
	"net/http"
	"time"

	"github.com/xleaks-org/xleaks/pkg/identity"
)

type createIdentityRequest struct {
	Passphrase string `json:"passphrase"`
}

type importIdentityRequest struct {
	Mnemonic   string `json:"mnemonic"`
	SeedPhrase string `json:"seedPhrase"`
	Passphrase string `json:"passphrase"`
}

type unlockIdentityRequest struct {
	Passphrase string `json:"passphrase"`
}

type switchIdentityRequest struct {
	Passphrase string `json:"passphrase"`
}

// CreateIdentity handles POST /api/identity/create.
func (h *Handler) CreateIdentity(w http.ResponseWriter, r *http.Request) {
	var req createIdentityRequest
	if err := parseJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Passphrase == "" {
		respondError(w, http.StatusBadRequest, "passphrase is required")
		return
	}

	if len(req.Passphrase) < 8 {
		respondError(w, http.StatusBadRequest, "passphrase must be at least 8 characters")
		return
	}

	if h.identity == nil {
		respondError(w, http.StatusInternalServerError, "identity system not initialized")
		return
	}

	kp, mnemonic, err := h.identity.CreateAndSave(req.Passphrase)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create identity: "+err.Error())
		return
	}

	// Update the handler's key pair reference and propagate to all services.
	h.updateIdentity(kp)

	pubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())
	address, _ := identity.PubKeyToAddress(kp.PublicKeyBytes())

	// Create a default profile in the database.
	h.db.UpsertProfile(kp.PublicKeyBytes(), DefaultDisplayName, "", nil, nil, "", 1, nowMillis())

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"pubkey":     pubkeyHex,
		"address":    address,
		"mnemonic":   mnemonic,
		"seedPhrase": mnemonic,
	})
}

// ImportIdentity handles POST /api/identity/import.
func (h *Handler) ImportIdentity(w http.ResponseWriter, r *http.Request) {
	var req importIdentityRequest
	if err := parseJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Accept both "mnemonic" and "seedPhrase" field names.
	mnemonic := req.Mnemonic
	if mnemonic == "" {
		mnemonic = req.SeedPhrase
	}

	if mnemonic == "" {
		respondError(w, http.StatusBadRequest, "mnemonic or seedPhrase is required")
		return
	}

	if req.Passphrase == "" {
		respondError(w, http.StatusBadRequest, "passphrase is required")
		return
	}

	if h.identity == nil {
		respondError(w, http.StatusInternalServerError, "identity system not initialized")
		return
	}

	kp, err := h.identity.ImportAndSave(mnemonic, req.Passphrase)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to import identity: "+err.Error())
		return
	}

	h.kp = kp

	pubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())
	address, _ := identity.PubKeyToAddress(kp.PublicKeyBytes())

	h.db.UpsertProfile(kp.PublicKeyBytes(), DefaultDisplayName, "", nil, nil, "", 1, nowMillis())

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"pubkey":  pubkeyHex,
		"address": address,
		"status":  "imported",
	})
}

// UnlockIdentity handles POST /api/identity/unlock.
func (h *Handler) UnlockIdentity(w http.ResponseWriter, r *http.Request) {
	var req unlockIdentityRequest
	if err := parseJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Passphrase == "" {
		respondError(w, http.StatusBadRequest, "passphrase is required")
		return
	}

	if h.identity == nil {
		respondError(w, http.StatusInternalServerError, "identity system not initialized")
		return
	}

	kp, err := h.identity.Unlock(req.Passphrase)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "failed to unlock: "+err.Error())
		return
	}

	h.kp = kp

	pubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())
	address, _ := identity.PubKeyToAddress(kp.PublicKeyBytes())

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "unlocked",
		"pubkey":  pubkeyHex,
		"address": address,
	})
}

// GetActiveIdentity handles GET /api/identity/active.
func (h *Handler) GetActiveIdentity(w http.ResponseWriter, r *http.Request) {
	if h.identity != nil && !h.identity.IsUnlocked() {
		// Identity exists on disk but not unlocked yet.
		if h.identity.HasIdentity() {
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"active": false,
				"locked": true,
			})
			return
		}
		// No identity at all.
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"active":         false,
			"needsOnboarding": true,
		})
		return
	}

	kp := h.kp
	if kp == nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"active":         false,
			"needsOnboarding": true,
		})
		return
	}

	pubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())
	address, _ := identity.PubKeyToAddress(kp.PublicKeyBytes())

	// Get profile from DB.
	profile, _ := h.db.GetProfile(kp.PublicKeyBytes())
	displayName := DefaultDisplayName
	if profile != nil {
		displayName = profile.DisplayName
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"active":      true,
		"pubkey":      pubkeyHex,
		"address":     address,
		"displayName": displayName,
	})
}

// LockIdentity handles POST /api/identity/lock.
func (h *Handler) LockIdentity(w http.ResponseWriter, r *http.Request) {
	if h.identity != nil {
		h.identity.Lock()
	}
	h.kp = nil
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "locked",
	})
}

// ListIdentities handles GET /api/identity/list.
func (h *Handler) ListIdentities(w http.ResponseWriter, r *http.Request) {
	if h.identity == nil {
		respondJSON(w, http.StatusOK, []interface{}{})
		return
	}

	identities, err := h.identity.ListIdentities()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list identities: "+err.Error())
		return
	}

	if len(identities) == 0 {
		respondJSON(w, http.StatusOK, []interface{}{})
		return
	}

	respondJSON(w, http.StatusOK, identities)
}

// SwitchIdentity handles PUT /api/identity/switch/{pubkey}.
func (h *Handler) SwitchIdentity(w http.ResponseWriter, r *http.Request) {
	pubkeyBytes, err := parseHexParam(r, "pubkey")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	pubkeyHex := hex.EncodeToString(pubkeyBytes)

	// Read passphrase from request body.
	var req switchIdentityRequest
	if err := parseJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Passphrase == "" {
		respondError(w, http.StatusBadRequest, "passphrase is required")
		return
	}

	if h.identity == nil {
		respondError(w, http.StatusInternalServerError, "identity system not initialized")
		return
	}

	if err := h.identity.SwitchIdentity(pubkeyHex, req.Passphrase); err != nil {
		respondError(w, http.StatusUnauthorized, "failed to switch identity: "+err.Error())
		return
	}

	// Update the handler's key pair reference.
	h.kp = h.identity.Get()

	address, _ := identity.PubKeyToAddress(pubkeyBytes)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "switched",
		"pubkey":  pubkeyHex,
		"address": address,
	})
}

// ExportIdentity handles GET /api/identity/export.
func (h *Handler) ExportIdentity(w http.ResponseWriter, r *http.Request) {
	kp := h.kp
	if kp == nil && h.identity != nil {
		kp = h.identity.Get()
	}
	if kp == nil {
		respondError(w, http.StatusNotFound, "no active identity")
		return
	}

	pubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())
	address, _ := identity.PubKeyToAddress(kp.PublicKeyBytes())

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"pubkey":  pubkeyHex,
		"address": address,
	})
}

func nowMillis() int64 {
	return time.Now().UnixMilli()
}
