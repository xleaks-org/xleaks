package web

import (
	"net/http"
)

// termsPage renders the terms acceptance page.
func (h *Handler) termsPage(w http.ResponseWriter, r *http.Request) {
	data := h.pageData(r, "", "Terms")
	h.renderPage(w, "terms.html", data)
}

// handleAcceptTerms processes the terms acceptance form.
func (h *Handler) handleAcceptTerms(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	node := r.FormValue("node")
	permanent := r.FormValue("permanent")
	tos := r.FormValue("tos")

	if node != "on" || permanent != "on" || tos != "on" {
		data := h.pageData(r, "", "Terms")
		data["Error"] = "You must accept all terms to continue."
		h.renderPage(w, "terms.html", data)
		return
	}

	// Get current user's pubkey from session
	sess := h.sessions.GetFromRequest(r)
	if sess == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	h.db.AcceptTerms(sess.KeyPair.PublicKeyBytes(), "1.0")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
