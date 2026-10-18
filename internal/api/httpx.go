package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/praetorianer777/gotree/internal/store"
)

// maxBody bounds JSON request bodies; uploads have their own limits.
const maxBody = 1 << 20

// errorBody is the JSON shape of every error response. Error is the English
// message (kept for older clients); Code and Params let the client translate
// it. Fields and FieldCodes carry per-field validation messages of a 422.
type errorBody struct {
	Error      string                `json:"error"`
	Code       string                `json:"code"`
	Params     map[string]any        `json:"params,omitempty"`
	Fields     map[string]string     `json:"fields,omitempty"`
	FieldCodes map[string]FieldError `json:"fieldCodes,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, errorBody{Error: msg, Code: code})
}

// writeErrorP is writeError with parameters for the translated message.
func writeErrorP(w http.ResponseWriter, status int, code, msg string, params map[string]any) {
	writeJSON(w, status, errorBody{Error: msg, Code: code, Params: params})
}

func validationBody(ve *store.ValidationError) errorBody {
	codes := make(map[string]FieldError, len(ve.Fields))
	for f, msg := range ve.Fields {
		codes[f] = classifyField(msg)
	}
	return errorBody{Error: "invalid input", Code: CodeInvalidInput, Fields: ve.Fields, FieldCodes: codes}
}

// writeStoreError maps store errors to HTTP responses; anything unexpected
// is logged and reported as a 500 without details.
func writeStoreError(w http.ResponseWriter, log *slog.Logger, err error) {
	var ve *store.ValidationError
	switch {
	case errors.As(err, &ve):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(ve))
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, CodeNotFound, "not found")
	case errors.Is(err, store.ErrViewOnly):
		writeError(w, http.StatusForbidden, CodeViewOnly, err.Error())
	case errors.Is(err, store.ErrOwnerOnly):
		writeError(w, http.StatusForbidden, CodeOwnerOnly, err.Error())
	case errors.Is(err, store.ErrApplied):
		writeError(w, http.StatusConflict, CodeTranscriptApplied, err.Error())
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, CodeStillInUse, "still in use")
	default:
		log.Error("request failed", "err", err)
		writeError(w, http.StatusInternalServerError, CodeInternal, "internal error")
	}
}

// decode reads a JSON body into v, rejecting unknown fields so typos in
// client code fail loudly instead of being silently dropped.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeDecodeError(w, err)
		return false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		if err != nil && writeBodyReadError(w, err) {
			return false
		}
		writeError(w, http.StatusBadRequest, CodeTrailingData, "invalid JSON: trailing data")
		return false
	}
	return true
}

// writeBodyReadError answers a body that could not be read: 413 when it is
// over maxBody, 408 when the read deadline expired. It reports false for
// other errors.
func writeBodyReadError(w http.ResponseWriter, err error) bool {
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		writeErrorP(w, http.StatusRequestEntityTooLarge, CodeRequestTooLarge,
			fmt.Sprintf("the request is larger than %d KB", tooBig.Limit>>10), map[string]any{"limitKB": tooBig.Limit >> 10})
	case errors.Is(err, os.ErrDeadlineExceeded):
		writeError(w, http.StatusRequestTimeout, CodeRequestTimeout, "the request took too long to arrive; check the connection and try again")
	default:
		return false
	}
	return true
}

// writeDecodeError answers a JSON body that cannot be decoded. The detail
// names the JSON field at most, never the Go types behind it.
func writeDecodeError(w http.ResponseWriter, err error) {
	if writeBodyReadError(w, err) {
		return
	}
	detail := "malformed JSON"
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.Is(err, io.EOF):
		detail = "empty body"
	case errors.Is(err, io.ErrUnexpectedEOF):
		detail = "unexpected end of JSON"
	case errors.As(err, &typeErr) && typeErr.Field != "":
		detail = fmt.Sprintf("wrong type for field %q", typeErr.Field)
	case errors.As(err, &typeErr):
		detail = "wrong type"
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		detail = strings.TrimPrefix(err.Error(), "json: ") // `unknown field "x"`: a JSON name
	}
	writeErrorP(w, http.StatusBadRequest, CodeInvalidJSON, "invalid JSON: "+detail, map[string]any{"detail": detail})
}

// bodyReadTimeout bounds how long a request body may take to arrive on
// routes that are not uploads; a variable so tests can shrink it.
var bodyReadTimeout = 15 * time.Second

// bodyDeadline sets a short read deadline for requests with a body, so a
// client trickling a JSON body (to /api/auth/login, say) cannot hold the
// connection. The deadline is cleared once the body is read, because a
// deadline still set while a long handler runs would cancel its context.
// Upload handlers replace it with their own (limitUpload).
func bodyDeadline(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.Body != http.NoBody {
			rc := http.NewResponseController(w)
			// Unsupported by some ResponseWriters (tests); then nothing changes.
			if rc.SetReadDeadline(time.Now().Add(bodyReadTimeout)) == nil {
				r.Body = &clearDeadlineOnEOF{ReadCloser: r.Body, rc: rc}
			}
		}
		next.ServeHTTP(w, r)
	})
}

type clearDeadlineOnEOF struct {
	io.ReadCloser
	rc   *http.ResponseController
	done bool
}

func (b *clearDeadlineOnEOF) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if errors.Is(err, io.EOF) && !b.done {
		b.done = true
		_ = b.rc.SetReadDeadline(time.Time{})
	}
	return n, err
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, CodeNotFound, "not found")
		return 0, false
	}
	return id, true
}

func queryInt(r *http.Request, name string, fallback int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil {
		return fallback
	}
	return v
}
