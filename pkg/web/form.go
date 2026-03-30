package web

import (
	"errors"
	"net/http"
)

const maxFormBodyBytes = 1 << 20

var errFormBodyTooLarge = errors.New("form body too large")

func limitFormBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isSafeMethod(r.Method) && r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxFormBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

func parseRequestForm(r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return errFormBodyTooLarge
		}
		return err
	}
	return nil
}

func formBodyTooLarge(err error) bool {
	return errors.Is(err, errFormBodyTooLarge)
}
