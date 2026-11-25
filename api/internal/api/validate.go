package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	maxBodyBytes  = 64 << 10 // 64 KiB: far above 4000 chars of JSON, stops abuse
	maxUserLen    = 64
	maxRoomName   = 100
	maxMessageLen = 4000
	defaultLimit  = 50
	maxLimit      = 100
)

// parseUUIDv7 accepts only RFC 9562 version-7 UUIDs (time-ordered). v4 is rejected
// because before/after pagination relies on IDs sorting by creation time.
func parseUUIDv7(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil || len(s) != 36 { // len check rejects the urn:/{braced}/no-dash forms
		return uuid.Nil, fmt.Errorf("%q is not a UUID", s)
	}
	if id.Version() != 7 || id.Variant() != uuid.RFC4122 {
		return uuid.Nil, fmt.Errorf("%q is not a UUIDv7", s)
	}
	return id, nil
}

// // pathRoomID reads {roomId}. A malformed path ID is invalid_request (400).
// func pathRoomID(r *http.Request) (uuid.UUID, error) {
// 	id, err := parseUUIDv7(r.PathValue("roomId"))
// 	if err != nil {
// 		return uuid.Nil, badRequest(codeInvalidRequest, "roomId: "+err.Error())
// 	}
// 	return id, nil
// }

// // parsePage reads limit / before / after from the query string.
// func parsePage(r *http.Request, allowAfter bool) (store.Page, error) {
// 	q := r.URL.Query()
// 	p := store.Page{Limit: defaultLimit}

// 	if v := q.Get("limit"); v != "" {
// 		n, err := strconv.Atoi(v)
// 		if err != nil || n < 1 || n > maxLimit {
// 			return p, badRequest(codeInvalidRequest, fmt.Sprintf("limit must be an integer from 1 to %d", maxLimit))
// 		}
// 		p.Limit = n
// 	}
// 	cursor := func(name string) (*uuid.UUID, error) {
// 		v := q.Get(name)
// 		if v == "" {
// 			return nil, nil
// 		}
// 		id, err := parseUUIDv7(v)
// 		if err != nil {
// 			return nil, badRequest(codeInvalidCursor, name+": "+err.Error())
// 		}
// 		return &id, nil
// 	}
// 	var err error
// 	if p.Before, err = cursor("before"); err != nil {
// 		return p, err
// 	}
// 	if allowAfter {
// 		if p.After, err = cursor("after"); err != nil {
// 			return p, err
// 		}
// 		if p.Before != nil && p.After != nil {
// 			return p, badRequest(codeInvalidCursor, "before and after are mutually exclusive")
// 		}
// 	}
// 	return p, nil
// }

func parseBool(r *http.Request, name string) (bool, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return false, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, badRequest(codeInvalidRequest, name+" must be true or false")
	}
	return b, nil
}

// decodeJSON enforces: JSON content, size cap, no unknown fields
// (additionalProperties: false), and exactly one JSON value.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		return badRequest(codeInvalidRequest, "Content-Type must be application/json")
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return badRequest(codeInvalidRequest, "request body too large")
		}
		return badRequest(codeInvalidRequest, "invalid JSON body: "+err.Error())
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return badRequest(codeInvalidRequest, "body must contain a single JSON object")
	}
	return nil
}

// checkText validates a user-supplied string: length in characters (JSON Schema
// minLength/maxLength count characters, not bytes) and no control characters
// (Postgres also rejects NUL bytes in text, which would otherwise be a 500).
func checkText(field, s string, min, max int, allowNewlines bool) error {
	n := utf8.RuneCountInString(s)
	if n < min || n > max {
		return badRequest(codeInvalidRequest, fmt.Sprintf("%s must be %d to %d characters", field, min, max))
	}
	for _, c := range s {
		if unicode.IsControl(c) && !(allowNewlines && (c == '\n' || c == '\t' || c == '\r')) {
			return badRequest(codeInvalidRequest, field+" contains control characters")
		}
	}
	return nil
}
