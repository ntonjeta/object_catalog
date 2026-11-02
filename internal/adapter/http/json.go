package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"biblio-oggetto/internal/domain"
	"biblio-oggetto/internal/httpx"
)

const maxBodyBytes = 1 << 20

var (
	errInternal      = httpx.ErrInternal
	errNotFoundRoute = fmt.Errorf("%w: route not found", domain.ErrNotFound)
)

func writeJSON(w http.ResponseWriter, status int, value any) {
	httpx.WriteJSON(w, status, value)
}

func writeError(w http.ResponseWriter, err error) {
	httpx.WriteError(w, err)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(destination); err != nil {
		writeError(w, fmt.Errorf("%w: body must be valid JSON", domain.ErrInvalid))
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, fmt.Errorf("%w: body must contain a single JSON value", domain.ErrInvalid))
		return false
	}
	return true
}
