package handlers

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
)

// updateProfileRequest is the JSON body for PUT /api/profile.
type updateProfileRequest struct {
	DisplayName string `json:"display_name"`
	Bio         string `json:"bio"`
	Website     string `json:"website"`
	AvatarCID   string `json:"avatar_cid"`
	BannerCID   string `json:"banner_cid"`
}

// GetOwnProfile handles GET /api/profile.
func (h *Handler) GetOwnProfile(w http.ResponseWriter, r *http.Request) {
	profile, err := h.profiles.GetProfile(h.kp.PublicKeyBytes())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if profile == nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"pubkey":       hex.EncodeToString(h.kp.PublicKeyBytes()),
			"display_name": "",
			"bio":          "",
			"website":      "",
			"avatar_cid":   "",
			"banner_cid":   "",
			"version":      0,
		})
		return
	}

	respondJSON(w, http.StatusOK, profileToJSON(profile))
}

// UpdateProfile handles PUT /api/profile.
func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	var req updateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var avatarCID, bannerCID []byte
	var err error
	if req.AvatarCID != "" {
		avatarCID, err = hex.DecodeString(req.AvatarCID)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid avatar_cid hex")
			return
		}
	}
	if req.BannerCID != "" {
		bannerCID, err = hex.DecodeString(req.BannerCID)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid banner_cid hex")
			return
		}
	}

	// Check if profile exists; if not, create it; otherwise update.
	existing, err := h.profiles.GetProfile(h.kp.PublicKeyBytes())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var profile interface{}
	if existing == nil {
		profile, err = h.profiles.CreateProfile(r.Context(), req.DisplayName, req.Bio, req.Website, avatarCID, bannerCID)
	} else {
		profile, err = h.profiles.UpdateProfile(r.Context(), req.DisplayName, req.Bio, req.Website, avatarCID, bannerCID)
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, profile)
}

// GetUserProfile handles GET /api/users/{pubkey}.
func (h *Handler) GetUserProfile(w http.ResponseWriter, r *http.Request) {
	pubkey, err := parseHexParam(r, "pubkey")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	profile, err := h.profiles.GetProfile(pubkey)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if profile == nil {
		respondError(w, http.StatusNotFound, "profile not found")
		return
	}

	respondJSON(w, http.StatusOK, profileToJSON(profile))
}

// profileToJSON converts a protobuf Profile to a JSON-friendly map.
func profileToJSON(profile interface{ GetAuthor() []byte }) map[string]interface{} {
	// Use type assertion to access profile fields.
	type profileData interface {
		GetAuthor() []byte
		GetDisplayName() string
		GetBio() string
		GetWebsite() string
		GetAvatarCid() []byte
		GetBannerCid() []byte
		GetVersion() uint64
		GetTimestamp() uint64
	}

	p, ok := profile.(profileData)
	if !ok {
		return nil
	}

	return map[string]interface{}{
		"pubkey":       hex.EncodeToString(p.GetAuthor()),
		"display_name": p.GetDisplayName(),
		"bio":          p.GetBio(),
		"website":      p.GetWebsite(),
		"avatar_cid":   hexOrEmpty(p.GetAvatarCid()),
		"banner_cid":   hexOrEmpty(p.GetBannerCid()),
		"version":      p.GetVersion(),
		"timestamp":    p.GetTimestamp(),
	}
}
