package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

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
		writeErrorP(w, http.StatusBadRequest, CodeInvalidJSON, "invalid JSON: "+err.Error(), map[string]any{"detail": err.Error()})
		return false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, CodeTrailingData, "invalid JSON: trailing data")
		return false
	}
	return true
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
