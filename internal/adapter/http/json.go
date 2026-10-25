package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"biblio-oggetto/internal/domain"
)

const maxBodyBytes = 1 << 20

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

var (
	errInternal      = errors.New("internal server error")
	errNotFoundRoute = fmt.Errorf("%w: route not found", domain.ErrNotFound)
)

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, err error) {
	status, code, message := httpStatus(err)
	if status == http.StatusInternalServerError {
		log.Printf("internal error: %v", err)
		message = "internal server error"
	}
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

func httpStatus(err error) (int, string, string) {
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
