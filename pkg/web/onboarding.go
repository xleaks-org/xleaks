package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/xleaks-org/xleaks/pkg/social"
)

// onboardingPage serves the create/import identity page.
func (h *Handler) onboardingPage(w http.ResponseWriter, r *http.Request) {
	if h.sessions.GetFromRequest(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if h.identity.IsUnlocked() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	data := h.pageData(r, "", "Get Started")
	if h.identity.HasIdentity() {
		data["Locked"] = true
	} else {
		data["NeedsOnboarding"] = true
	}
	h.renderPage(w, "onboarding.html", data)
}

// handleCreate processes the identity creation form.
func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	if err := parseRequestForm(r); err != nil {
		if formBodyTooLarge(err) {
			http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
			return
		}
		h.renderOnboardingError(w, r, "Invalid form data", true)
		return
	}
	passphrase := r.FormValue("passphrase")
	confirm := r.FormValue("confirm")
	minLen := h.passphraseMinLen()
	if len(passphrase) < minLen {
		h.renderOnboardingError(w, r, fmt.Sprintf("Passphrase must be at least %d characters", minLen), true)
		return
	}
	if passphrase != confirm {
		h.renderOnboardingError(w, r, "Passphrases do not match", true)
		return
	}
	kp, mnemonic, err := h.identity.CreateAndSave(passphrase)
	if err != nil {
		h.renderOnboardingError(w, r, fmt.Sprintf("Failed to create identity: %v", err), true)
		return
	}
	h.notifyIdentityChange()
	h.ensureProfile()

	// Create a session for the new identity.
	token, err := h.sessions.Create(kp)
	if err != nil {
		slog.Error("failed to create session after identity create", "error", err)
	} else {
		h.sessions.SetCookie(w, r, token)
	}

	data := h.pageData(r, "", "Save Seed Phrase")
	data["SeedPhrase"] = mnemonic
	data["SeedWords"] = strings.Fields(mnemonic)
	h.renderPage(w, "onboarding.html", data)
}

// handleVerifyStep shows the seed confirmation page.
func (h *Handler) handleVerifyStep(w http.ResponseWriter, r *http.Request) {
	if err := parseRequestForm(r); err != nil {
		if formBodyTooLarge(err) {
			http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	seed := r.FormValue("seed")
	words := strings.Fields(seed)
	if len(words) != seedPhraseLength {
		h.renderOnboardingError(w, r, "Invalid seed phrase", true)
		return
	}
	h.renderSeedConfirmPage(w, r, seed, words, "")
}

// handleConfirmSeed verifies the user filled in the correct words.
func (h *Handler) handleConfirmSeed(w http.ResponseWriter, r *http.Request) {
	if err := parseRequestForm(r); err != nil {
		if formBodyTooLarge(err) {
			http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	seed := r.FormValue("seed")
	words := strings.Fields(seed)
	posStr := r.FormValue("blank_positions")
	posParts := strings.Split(posStr, ",")

	allCorrect := true
	for _, ps := range posParts {
		pos, err := strconv.Atoi(strings.TrimSpace(ps))
		if err != nil || pos >= len(words) {
			allCorrect = false
			break
		}
		entered := strings.TrimSpace(strings.ToLower(r.FormValue(fmt.Sprintf("word_%d", pos))))
		if entered != words[pos] {
			allCorrect = false
			break
		}
	}
	if !allCorrect {
		h.renderSeedConfirmPage(w, r, seed, words, "Some words don't match. Try again.")
		return
	}
	data := h.pageData(r, "", "Set Your Name")
	data["SetProfile"] = true
	h.renderPage(w, "onboarding.html", data)
}

// handleSetProfile sets the user's display name.
func (h *Handler) handleSetProfile(w http.ResponseWriter, r *http.Request) {
	if err := parseRequestForm(r); err != nil {
		if formBodyTooLarge(err) {
			http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	displayName := strings.TrimSpace(r.FormValue("display_name"))
	if displayName == "" {
		displayName = "Anonymous"
	}
	if err := social.ValidateProfileFields(displayName, "", ""); err != nil {
		data := h.pageData(r, "", "Set Your Name")
		data["SetProfile"] = true
		data["Error"] = err.Error()
		h.renderPage(w, "onboarding.html", data)
		return
	}

	// Try session key pair first, then fall back to global identity.
	sess := h.sessions.GetFromRequest(r)
	if sess != nil {
		if err := h.db.UpsertProfile(sess.KeyPair.PublicKeyBytes(), displayName, "", nil, nil, "", 2, time.Now().UnixMilli()); err != nil {
			slog.Error("failed to upsert profile", "error", err)
		}
	} else if h.identity.IsUnlocked() {
		kp := h.identity.Get()
		if kp != nil {
			// Use version 2 to ensure it overwrites the default "Anonymous" profile (version 1)
			if err := h.db.UpsertProfile(kp.PublicKeyBytes(), displayName, "", nil, nil, "", 2, time.Now().UnixMilli()); err != nil {
				slog.Error("failed to upsert profile", "error", err)
			}
		}
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleImport processes the identity import form.
func (h *Handler) handleImport(w http.ResponseWriter, r *http.Request) {
	if err := parseRequestForm(r); err != nil {
		if formBodyTooLarge(err) {
			http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
			return
		}
		h.renderOnboardingError(w, r, "Invalid form data", true)
		return
	}
	mnemonic := strings.TrimSpace(r.FormValue("mnemonic"))
	passphrase := r.FormValue("passphrase")
	if mnemonic == "" {
		h.renderOnboardingError(w, r, "Seed phrase is required", true)
		return
	}
	minLen := h.passphraseMinLen()
	if len(passphrase) < minLen {
		h.renderOnboardingError(w, r, fmt.Sprintf("Passphrase must be at least %d characters", minLen), true)
		return
	}
	kp, err := h.identity.ImportAndSave(mnemonic, passphrase)
	if err != nil {
		h.renderOnboardingError(w, r, fmt.Sprintf("Failed to import identity: %v", err), true)
		return
	}
	h.notifyIdentityChange()
	h.ensureProfile()

	// Create a session for the imported identity.
	token, err := h.sessions.Create(kp)
	if err != nil {
		slog.Error("failed to create session after identity import", "error", err)
	} else {
		h.sessions.SetCookie(w, r, token)
	}

	// Only ask for name if profile has default "Anonymous" name
	profile, _ := h.db.GetProfile(kp.PublicKeyBytes())
	if profile != nil && profile.DisplayName != "" && profile.DisplayName != "Anonymous" {
		// Profile already has a real name -- go straight to home
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	// First time on this server -- ask for name
	data := h.pageData(r, "", "Set Your Name")
	data["SetProfile"] = true
	h.renderPage(w, "onboarding.html", data)
}

// handleUnlock processes the unlock form.
func (h *Handler) handleUnlock(w http.ResponseWriter, r *http.Request) {
	if err := parseRequestForm(r); err != nil {
		if formBodyTooLarge(err) {
			http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
			return
		}
		h.renderOnboardingError(w, r, "Invalid form data", false)
		return
	}
	passphrase := r.FormValue("passphrase")
	if passphrase == "" {
		h.renderOnboardingError(w, r, "Passphrase is required", false)
		return
	}
	kp, err := h.identity.Unlock(passphrase)
	if err != nil {
		data := h.pageData(r, "", "Unlock")
		data["Locked"] = true
		data["Error"] = "Incorrect passphrase"
		h.renderPage(w, "onboarding.html", data)
		return
	}
	h.notifyIdentityChange()
	h.ensureProfile()

	// Create a session for the unlocked identity.
	token, err := h.sessions.Create(kp)
	if err != nil {
		slog.Error("failed to create session after unlock", "error", err)
	} else {
		h.sessions.SetCookie(w, r, token)
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleLogout destroys the session and redirects to the landing page.
func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if h.identity != nil {
		h.identity.Lock()
		h.notifyIdentityChange()
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		h.sessions.Destroy(cookie.Value)
	}
	h.sessions.ClearCookie(w, r)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// renderOnboardingError renders the onboarding page with an error message.
func (h *Handler) renderOnboardingError(w http.ResponseWriter, r *http.Request, errMsg string, needsOnboarding bool) {
	data := h.pageData(r, "", "Get Started")
	if needsOnboarding {
		data["NeedsOnboarding"] = true
	} else {
		data["Locked"] = true
	}
	data["Error"] = errMsg
	h.renderPage(w, "onboarding.html", data)
}

// renderSeedConfirmPage renders the seed phrase confirmation page.
func (h *Handler) renderSeedConfirmPage(w http.ResponseWriter, r *http.Request, seed string, words []string, errMsg string) {
	positions := pickRandomPositions(seedPhraseLength, 3)
	slots, posStr := buildWordSlots(words, positions)
	data := h.pageData(r, "", "Verify Seed Phrase")
	data["ConfirmSeed"] = true
	data["HiddenSeed"] = seed
	data["WordSlots"] = slots
	data["BlankPositions"] = posStr
	if errMsg != "" {
		data["Error"] = errMsg
	}
	h.renderPage(w, "onboarding.html", data)
}
