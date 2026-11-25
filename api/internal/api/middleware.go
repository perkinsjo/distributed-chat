package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"
)

type ctxKey int

const userKey ctxKey = iota

// userFrom returns the caller identity set by withIdentity.
func userFrom(r *http.Request) string {
	u, _ := r.Context().Value(userKey).(string)
	return u
}

// withIdentity enforces the spec's identity rule: X-User-ID header, or ?user= for
// clients that cannot set headers. Missing -> 400 missing_identity.
func (s *Server) withIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Header.Get("X-User-ID")
		if user == "" {
			user = r.URL.Query().Get("user")
		}
		if user == "" {
			writeError(w, http.StatusBadRequest, codeMissingIdentity, "set the X-User-ID header or the user query parameter")
			return
		}
		if err := checkText("user id", user, 1, maxUserLen, false); err != nil {
			_, code, msg := toHTTP(err)
			writeError(w, http.StatusBadRequest, code, msg)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, user)))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// withLogging writes one structured JSON line per request. CloudWatch Logs Insights can
// compute p50/p95/p99 per route straight from these fields.
func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rid := r.Header.Get("X-Request-ID")
		if rid == "" {
			rid = newRequestID()
		}
		w.Header().Set("X-Request-ID", rid)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		if r.URL.Path == "/healthz" && rec.status == http.StatusOK {
			return // the ALB hits this every few seconds; don't flood the logs
		}
		s.log.Info("request",
			"request_id", rid,
			"method", r.Method,
			"route", r.Pattern, // e.g. "POST /rooms/{roomId}/messages", set by ServeMux
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", float64(time.Since(start).Microseconds())/1000,
			"user", r.Header.Get("X-User-ID"),
		)
	})
}

// withRecover turns a panic into a 500 instead of killing the process.
func (s *Server) withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.log.Error("panic", "err", v, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, codeInternal, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
