package api

import (
	"encoding/json"
	"errors"
	"net/http"
)

const (
	codeInvalidRequest  = "invalid_request"
	codeMissingIdentity = "missing_identity"
	codeInvalidCursor   = "invalid_cursor"
	codeRoomNotFound    = "room_not_found"
	codeIDConflict      = "id_conflict"
	codeUnavailable     = "unavailable"
	codeInternal        = "internal"
)

// apiError is a client-facing error a handler can return directly.
type apiError struct {
	status  int
	code    string
	message string
}

func (e *apiError) Error() string { return e.code + ": " + e.message }

func badRequest(code, msg string) error {
	return &apiError{status: http.StatusBadRequest, code: code, message: msg}
}

// handlerFunc is our handler shape: return an error instead of writing it.
type handlerFunc func(w http.ResponseWriter, r *http.Request) error

func (s *Server) handle(h handlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			status, code, msg := toHTTP(err)
			if status >= 500 {
				s.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
			}
			writeError(w, status, code, msg)
		}
	})
}

func toHTTP(err error) (int, string, string) {
	var ae *apiError
	switch {
	case errors.As(err, &ae):
		return ae.status, ae.code, ae.message
	// case errors.Is(err, store.ErrNotFound):
	// 	return http.StatusNotFound, codeRoomNotFound,
	// 		"room not found (if it was just created, retry in a few seconds)"
	// case errors.Is(err, store.ErrConflict):
	// 	return http.StatusConflict, codeIDConflict, "a resource with this id already exists with different contents"
	// case errors.Is(err, store.ErrUnavailable):
	// 	return http.StatusServiceUnavailable, codeUnavailable, "no database node is reachable, retry later"
	default:
		return http.StatusInternalServerError, codeInternal, "internal error"
	}
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	type body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	writeJSON(w, status, map[string]body{"error": {Code: code, Message: msg}})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// createdOrOK picks 201 for a new resource and 200 for an idempotent replay.
func createdOrOK(created bool) int {
	if created {
		return http.StatusCreated
	}
	return http.StatusOK
}
