package api

import (
	"regexp"
	"strconv"
	"strings"
)

// Stable machine codes for API errors. The web app translates them through
// the i18n keys "apiError.<code>"; the English message stays in the response
// for older clients and for codes a client does not know. Never rename or
// reuse a code: add a new one instead.
const (
	// Request-level errors.
	CodeInvalidJSON       = "invalid_json"
	CodeTrailingData      = "trailing_data"
	CodeInvalidInput      = "invalid_input"
	CodeNotFound          = "not_found"
	CodeMethodNotAllowed  = "method_not_allowed"
	CodeInternal          = "internal_error"
	CodeStillInUse        = "still_in_use"
	CodeOwnerOnly         = "owner_only"
	CodeTranscriptApplied = "transcription_applied"
	CodeInvalidScope      = "invalid_scope"
	CodeIDsRequired       = "ids_required"
	CodeEntityIDRequired  = "entity_id_required"

	// Auth and membership.
	CodeNotLoggedIn     = "not_logged_in"
	CodeNotMember       = "not_member"
	CodeViewOnly        = "view_only"
	CodeBadCredentials  = "bad_credentials"
	CodeTooManyLogins   = "too_many_logins"
	CodeSetupDone       = "setup_done"
	CodeAdminOnlyBackup = "admin_only_backup"

	// Sharing.
	CodeShareReadOnly = "share_read_only"
	CodeShareGone     = "share_gone"

	// Uploads, media and GEDCOM.
	CodeExpectedMultipart = "expected_multipart"
	CodeInvalidUpload     = "invalid_upload"
	CodeNoFile            = "no_file"
	CodeFileTooLarge      = "file_too_large"
	CodeUploadTimeout     = "upload_timeout"
	CodeUnsupportedMedia  = "unsupported_media"
	CodeImageTooLarge     = "image_too_large"
	CodeStoreFileFailed   = "store_file_failed"
	CodeFileMissing       = "file_missing"
	CodeNoThumbnail       = "no_thumbnail"
	CodeThumbTooLarge     = "thumbnail_too_large"
	CodeThumbFailed       = "thumbnail_failed"
	CodeReplaceOwnerOnly  = "replace_owner_only"
	CodeGedcomTooLarge    = "gedcom_too_large"
	CodeGedcomInvalid     = "gedcom_invalid"
	CodeTreeNotEmpty      = "tree_not_empty"
)

// Codes of per-field validation messages (the "fieldCodes" of a 422).
const (
	FieldInvalid        = "invalid"
	FieldRequired       = "required"
	FieldTooLong        = "too_long"
	FieldNotFound       = "not_found"
	FieldInvalidDate    = "invalid_date"
	FieldDateInPast     = "date_in_past"
	FieldInvalidChoice  = "invalid_choice"
	FieldUnknownValue   = "unknown_value"
	FieldOutOfRange     = "out_of_range"
	FieldTooMany        = "too_many"
	FieldDuplicate      = "duplicate"
	FieldInvalidURL     = "invalid_url"
	FieldSelfReference  = "self_reference"
	FieldCycle          = "cycle"
	FieldNameRequired   = "name_required"
	FieldPickPerson     = "pick_person"
	FieldPasswordShort  = "password_too_short"
	FieldPasswordLong   = "password_too_long"
	FieldUsernameFormat = "username_format"
	FieldCoordsTogether = "coords_together"
	FieldReasonRequired = "reason_required"
	FieldCustomLabel    = "custom_label_required"
	FieldFamilyChoice   = "family_choice"
)

// FieldError is the machine-readable form of one validation message.
type FieldError struct {
	Code   string         `json:"code"`
	Params map[string]any `json:"params,omitempty"`
}

var (
	reTooLong    = regexp.MustCompile(`(?i)\btoo long\b`)
	reNotExist   = regexp.MustCompile(`does not exist$`)
	reBetween    = regexp.MustCompile(`^must be between (-?\d+) and (-?\d+)$`)
	reMustBe     = regexp.MustCompile(`^must be (.+)$`)
	reUnknown    = regexp.MustCompile(`^unknown (?:[a-z ]+: |kind |status |role |action |column |name type |template |participant role |way of getting it: )?(.+)$`)
	reTooMany    = regexp.MustCompile(`^too many\b`)
	reDuplicated = regexp.MustCompile(`listed twice|repeated`)
)

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// classifyField derives the code and params of a store validation message.
// The store keeps plain English messages; this is the single place that maps
// them to stable codes. Anything not recognised becomes FieldInvalid and the
// client shows the English message.
func classifyField(msg string) FieldError {
	switch msg {
	case "is required":
		return FieldError{Code: FieldRequired}
	case "must be a date":
		return FieldError{Code: FieldInvalidDate}
	case "must not be in the past":
		return FieldError{Code: FieldDateInPast}
	case "must be at least 10 characters":
		return FieldError{Code: FieldPasswordShort, Params: map[string]any{"count": 10}}
	case "must be at most 72 bytes":
		return FieldError{Code: FieldPasswordLong, Params: map[string]any{"count": 72}}
	case "use 2–64 letters, digits, dots, dashes or underscores":
		return FieldError{Code: FieldUsernameFormat}
	case "latitude and longitude must be given together":
		return FieldError{Code: FieldCoordsTogether}
	case "must start with http:// or https://":
		return FieldError{Code: FieldInvalidURL}
	case "a person cannot be their own relative", "a person cannot be their own partner",
		"a place cannot be inside itself", "the partners must be two different people",
		"the principal cannot also be a participant":
		return FieldError{Code: FieldSelfReference}
	case "a person cannot be a child of themselves or of their own descendant",
		"a person cannot be a parent of their own ancestor":
		return FieldError{Code: FieldCycle}
	case "pick the person", "pick the person whose descendants to share":
		return FieldError{Code: FieldPickPerson}
	case "a new person needs a name", "a template needs a name column", "a custom role needs a name":
		return FieldError{Code: FieldNameRequired}
	case "is required for custom events":
		return FieldError{Code: FieldCustomLabel}
	case "this person has several families; choose one", "is not one of this person's families":
		return FieldError{Code: FieldFamilyChoice}
	case "say why the fact is disputed or disproven", "say why the name is disputed or disproven":
		return FieldError{Code: FieldReasonRequired}
	}
	if m := reBetween.FindStringSubmatch(msg); m != nil {
		return FieldError{Code: FieldOutOfRange, Params: map[string]any{"min": atoi(m[1]), "max": atoi(m[2])}}
	}
	if reTooLong.MatchString(msg) {
		return FieldError{Code: FieldTooLong}
	}
	if reNotExist.MatchString(msg) {
		return FieldError{Code: FieldNotFound}
	}
	if reTooMany.MatchString(msg) {
		return FieldError{Code: FieldTooMany}
	}
	if reDuplicated.MatchString(msg) {
		return FieldError{Code: FieldDuplicate}
	}
	if m := reMustBe.FindStringSubmatch(msg); m != nil {
		return FieldError{Code: FieldInvalidChoice, Params: map[string]any{"options": strings.ReplaceAll(m[1], " or ", ", ")}}
	}
	if m := reUnknown.FindStringSubmatch(msg); m != nil {
		return FieldError{Code: FieldUnknownValue, Params: map[string]any{"value": m[1]}}
	}
	return FieldError{Code: FieldInvalid}
}
