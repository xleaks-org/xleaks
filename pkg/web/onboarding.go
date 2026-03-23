package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// onboardingPage serves the create/import identity page.
func (h *Handler) onboardingPage(w http.ResponseWriter, r *http.Request) {
	if h.identity.IsUnlocked() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	data := h.pageData("", "Get Started")
	if h.identity.HasIdentity() {
		data["Locked"] = true
	} else {
		data["NeedsOnboarding"] = true
	}
	h.renderPage(w, "onboarding.html", data)
}

// handleCreate processes the identity creation form.
func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderOnboardingError(w, "Invalid form data", true)
		return
	}
	passphrase := r.FormValue("passphrase")
	confirm := r.FormValue("confirm")
	if len(passphrase) < 8 {
		h.renderOnboardingError(w, "Passphrase must be at least 8 characters", true)
		return
	}
	if passphrase != confirm {
		h.renderOnboardingError(w, "Passphrases do not match", true)
		return
	}
	_, mnemonic, err := h.identity.CreateAndSave(passphrase)
	if err != nil {
		h.renderOnboardingError(w, fmt.Sprintf("Failed to create identity: %v", err), true)
		return
	}
	data := h.pageData("", "Save Seed Phrase")
	data["SeedPhrase"] = mnemonic
	data["SeedWords"] = strings.Fields(mnemonic)
	h.renderPage(w, "onboarding.html", data)
}

// handleVerifyStep shows the seed confirmation page.
func (h *Handler) handleVerifyStep(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	seed := r.FormValue("seed")
	words := strings.Fields(seed)
	if len(words) != seedPhraseLength {
		h.renderOnboardingError(w, "Invalid seed phrase", true)
		return
	}
	h.renderSeedConfirmPage(w, seed, words, "")
}

// handleConfirmSeed verifies the user filled in the correct words.
func (h *Handler) handleConfirmSeed(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
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
		h.renderSeedConfirmPage(w, seed, words, "Some words don't match. Try again.")
		return
	}
	data := h.pageData("", "Set Your Name")
	data["SetProfile"] = true
	h.renderPage(w, "onboarding.html", data)
}

// handleSetProfile sets the user's display name.
func (h *Handler) handleSetProfile(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	displayName := strings.TrimSpace(r.FormValue("display_name"))
	if displayName == "" {
		displayName = "Anonymous"
	}
	if h.identity.IsUnlocked() {
		kp := h.identity.Get()
		if kp != nil {
			h.db.UpsertProfile(kp.PublicKeyBytes(), displayName, "", nil, nil, "", 1, time.Now().UnixMilli())
		}
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleImport processes the identity import form.
func (h *Handler) handleImport(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderOnboardingError(w, "Invalid form data", true)
		return
	}
	mnemonic := strings.TrimSpace(r.FormValue("mnemonic"))
	passphrase := r.FormValue("passphrase")
	if mnemonic == "" {
		h.renderOnboardingError(w, "Seed phrase is required", true)
		return
	}
	if len(passphrase) < 8 {
		h.renderOnboardingError(w, "Passphrase must be at least 8 characters", true)
		return
	}
	_, err := h.identity.ImportAndSave(mnemonic, passphrase)
	if err != nil {
		h.renderOnboardingError(w, fmt.Sprintf("Failed to import identity: %v", err), true)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleUnlock processes the unlock form.
func (h *Handler) handleUnlock(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderOnboardingError(w, "Invalid form data", false)
		return
	}
	passphrase := r.FormValue("passphrase")
	if passphrase == "" {
		h.renderOnboardingError(w, "Passphrase is required", false)
		return
	}
	_, err := h.identity.Unlock(passphrase)
	if err != nil {
		data := h.pageData("", "Unlock")
		data["Locked"] = true
		data["Error"] = "Incorrect passphrase"
		h.renderPage(w, "onboarding.html", data)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleLogout locks the identity and redirects to onboarding.
func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	h.identity.Lock()
	http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
}

// renderOnboardingError renders the onboarding page with an error message.
func (h *Handler) renderOnboardingError(w http.ResponseWriter, errMsg string, needsOnboarding bool) {
	data := h.pageData("", "Get Started")
	if needsOnboarding {
		data["NeedsOnboarding"] = true
	} else {
		data["Locked"] = true
	}
	data["Error"] = errMsg
	h.renderPage(w, "onboarding.html", data)
}

// renderSeedConfirmPage renders the seed phrase confirmation page.
func (h *Handler) renderSeedConfirmPage(w http.ResponseWriter, seed string, words []string, errMsg string) {
	positions := pickRandomPositions(seedPhraseLength, 3)
	slots, posStr := buildWordSlots(words, positions)
	data := h.pageData("", "Verify Seed Phrase")
	data["ConfirmSeed"] = true
	data["HiddenSeed"] = seed
	data["WordSlots"] = slots
	data["BlankPositions"] = posStr
	if errMsg != "" {
		data["Error"] = errMsg
	}
	h.renderPage(w, "onboarding.html", data)
}
