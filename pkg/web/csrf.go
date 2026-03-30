package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
)

const (
	csrfCookieName = "xleaks_csrf"
	csrfHeaderName = "X-CSRF-Token"
	csrfTokenLen   = 32
)

type csrfContextKey struct{}

func ensureCSRFCookie(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := csrfTokenFromRequest(r)
		if !validCSRFToken(token) && isSafeMethod(r.Method) {
			var err error
			token, err = generateCSRFToken()
			if err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			setCSRFCookie(w, r, token)
		}
		if validCSRFToken(token) {
			r = r.WithContext(context.WithValue(r.Context(), csrfContextKey{}, token))
		}
		next.ServeHTTP(w, r)
	})
}

func requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isSafeMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie(csrfCookieName)
		if err != nil || !validCSRFToken(cookie.Value) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		token := strings.TrimSpace(r.Header.Get(csrfHeaderName))
		if token == "" {
			if err := parseRequestForm(r); err != nil {
				if formBodyTooLarge(err) {
					http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
					return
				}
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			token = strings.TrimSpace(r.FormValue("csrf_token"))
		}
		if !validCSRFToken(token) || subtle.ConstantTimeCompare([]byte(token), []byte(cookie.Value)) != 1 {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		r = r.WithContext(context.WithValue(r.Context(), csrfContextKey{}, cookie.Value))
		next.ServeHTTP(w, r)
	})
}

func csrfTokenFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if token, ok := r.Context().Value(csrfContextKey{}).(string); ok && validCSRFToken(token) {
		return token
	}
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil || !validCSRFToken(cookie.Value) {
		return ""
	}
	return cookie.Value
}

func generateCSRFToken() (string, error) {
	b := make([]byte, csrfTokenLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func setCSRFCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionMaxAge.Seconds()),
		HttpOnly: true,
		Secure:   requestIsSecure(r),
		SameSite: http.SameSiteStrictMode,
	})
}

func validCSRFToken(token string) bool {
	if len(token) != csrfTokenLen*2 {
		return false
	}
	_, err := hex.DecodeString(token)
	return err == nil
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}
