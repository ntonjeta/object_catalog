package httpx

import (
	"log"
	"net/http"
)

// RecoverPanics converts panics in the wrapped handler into the standard
// internal error response instead of killing the connection.
func RecoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("panic serving %s %s: %v", r.Method, r.URL.Path, recovered)
				WriteError(w, ErrInternal)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
