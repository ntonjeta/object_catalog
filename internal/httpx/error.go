// Package httpx carries wire-format helpers shared by the HTTP adapters
// (the catalog API and the ActivityPub federation adapter), keeping the
// public error envelope defined in one place.
package httpx

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"biblio-oggetto/internal/domain"
)

// ErrInternal is the sentinel for failures whose details must stay in the
// server log; clients only ever receive the generic message.
var ErrInternal = errors.New("internal server error")

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func WriteError(w http.ResponseWriter, err error) {
	status, code, message := Status(err)
	if status == http.StatusInternalServerError {
		log.Printf("internal error: %v", err)
		message = "internal server error"
	}
	WriteJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

// Status maps domain sentinel errors to the API error contract:
// invalid_input (400), unauthorized (401), not_found (404), conflict (409),
// internal (500).
func Status(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		return http.StatusBadRequest, "invalid_input", publicMessage(err)
	case errors.Is(err, domain.ErrUnauthorized):
		message := publicMessage(err)
		if message == "request failed" {
			message = "authentication required"
		}
		return http.StatusUnauthorized, "unauthorized", message
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "not_found", publicMessage(err)
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict, "conflict", publicMessage(err)
	default:
		return http.StatusInternalServerError, "internal", "internal server error"
	}
}

func publicMessage(err error) string {
	message := err.Error()
	switch {
	case errors.Is(err, domain.ErrInvalid):
		message = trimSentinel(message, domain.ErrInvalid.Error())
	case errors.Is(err, domain.ErrNotFound):
		message = trimSentinel(message, domain.ErrNotFound.Error())
	case errors.Is(err, domain.ErrConflict):
		message = trimSentinel(message, domain.ErrConflict.Error())
	case errors.Is(err, domain.ErrUnauthorized):
		message = trimSentinel(message, domain.ErrUnauthorized.Error())
	}
	if message == "" {
		return "request failed"
	}
	return message
}

func trimSentinel(message, sentinel string) string {
	if !strings.HasPrefix(message, sentinel) {
		return message
	}
	trimmed := message[len(sentinel):]
	for len(trimmed) > 0 && (trimmed[0] == ':' || trimmed[0] == ' ') {
		trimmed = trimmed[1:]
	}
	return trimmed
}
