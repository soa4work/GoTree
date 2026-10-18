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
	CodeRequestTooLarge   = "request_too_large"
	CodeRequestTimeout    = "request_timeout"
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
	CodeGedcomInvalid     = "gedcom_invalid" // fallback for a file error without a code of its own
	CodeGedcomNotGedcom   = "gedcom_not_gedcom"
	CodeGedcomEmpty       = "gedcom_empty"
	CodeGedcomZipBad      = "gedcom_zip_unreadable"
	CodeGedcomZipNoFile   = "gedcom_zip_no_ged"
	CodeGedcomZipSeveral  = "gedcom_zip_several_ged"
	CodeTreeNotEmpty      = "tree_not_empty"
)

// Codes of per-field validation messages (the "fieldCodes" of a 422).
const (
	FieldInvalid        = "invalid" // catch-all: the client shows the server's message
	FieldRequired       = "required"
	FieldTooLong        = "too_long"
	FieldNotFound       = "not_found"
	FieldInvalidDate    = "invalid_date"
	FieldDateInPast     = "date_in_past"
	FieldDateOutOfRange = "date_out_of_range"
	FieldNotCanonical   = "not_canonical_date"
	FieldInvalidChoice  = "invalid_choice"
	FieldInvalidTag     = "invalid_tag"
	FieldUnknown        = "unknown"
	FieldUnknownValue   = "unknown_value"
	FieldOutOfRange     = "out_of_range"
	FieldTooMany        = "too_many"
	FieldTooManyRows    = "too_many_rows"
	FieldDuplicate      = "duplicate"
	FieldInvalidURL     = "invalid_url"
	FieldSelfReference  = "self_reference"
	FieldCycle          = "cycle"
	FieldNameRequired   = "name_required"
	FieldNamePart       = "name_part_required"
	FieldNameLength     = "name_length"
	FieldTextLength     = "text_length"
	FieldRecordTitle    = "record_title"
	FieldPickPerson     = "pick_person"
	FieldPickSource     = "pick_source"
	FieldPersonOrNew    = "person_or_new"
	FieldPasswordShort  = "password_too_short"
	FieldPasswordLong   = "password_too_long"
	FieldWrongPassword  = "wrong_password"
	FieldUsernameFormat = "username_format"
	FieldCoordsTogether = "coords_together"
	FieldReasonRequired = "reason_required"
	FieldCustomLabel    = "custom_label_required"
	FieldFamilyChoice   = "family_choice"
	FieldParentsKnown   = "parents_known"
	FieldEventOwner     = "event_owner"
	FieldFamilyEvent    = "family_event"
	FieldLivingNoDeath  = "living_no_death"
	FieldPlaceComma     = "place_comma"
	FieldPlaceEmpty     = "place_empty"
	FieldHolderRequired = "holder_required"
	FieldPhotoRequired  = "photo_required"
	FieldNotFaceTag     = "not_face_tag"
	FieldOutsideImage   = "outside_image"
	FieldNotSuggestion  = "not_suggestion"
	FieldNotAttached    = "citation_not_attached"
	FieldRolesInvalid   = "roles_invalid"
	FieldRowsInvalid    = "rows_invalid"
)

// FieldError is the machine-readable form of one validation message.
type FieldError struct {
	Code   string         `json:"code"`
	Params map[string]any `json:"params,omitempty"`
}

// exactFields maps messages that have no variable part to their code.
var exactFields = map[string]string{
	"is required":                    FieldRequired,
	"must be a date":                 FieldInvalidDate,
	"is not your current password":   FieldWrongPassword,
	"must not be in the past":        FieldDateInPast,
	"month and day are out of range": FieldDateOutOfRange,
	"use 2–64 letters, digits, dots, dashes or underscores":               FieldUsernameFormat,
	"latitude and longitude must be given together":                       FieldCoordsTogether,
	"must start with http:// or https://":                                 FieldInvalidURL,
	"a person cannot be their own relative":                               FieldSelfReference,
	"a person cannot be their own partner":                                FieldSelfReference,
	"a place cannot be inside itself":                                     FieldSelfReference,
	"the partners must be two different people":                           FieldSelfReference,
	"the principal cannot also be a participant":                          FieldSelfReference,
	"a person cannot be a child of themselves or of their own descendant": FieldCycle,
	"a person cannot be a parent of their own ancestor":                   FieldCycle,
	"pick the person": FieldPickPerson,
	"pick the person whose descendants to share":                           FieldPickPerson,
	"pick the source the record comes from":                                FieldPickSource,
	"give either an existing person or a new one":                          FieldPersonOrNew,
	"a new person needs a name":                                            FieldNameRequired,
	"a template needs a name column":                                       FieldNameRequired,
	"a custom role needs a name":                                           FieldNameRequired,
	"a name needs at least a given name, surname or nickname":              FieldNamePart,
	"name the kind of record, e.g. “School register”":                      FieldRecordTitle,
	"is required for custom events":                                        FieldCustomLabel,
	"this person has several families; choose one":                         FieldFamilyChoice,
	"is not one of this person's families":                                 FieldFamilyChoice,
	"say why the fact is disputed or disproven":                            FieldReasonRequired,
	"say why the name is disputed or disproven":                            FieldReasonRequired,
	"both parents in this family are already known":                        FieldParentsKnown,
	"an event belongs to exactly one person or one family":                 FieldEventOwner,
	"this event belongs to a family":                                       FieldFamilyEvent,
	"a living person has no death":                                         FieldLivingNoDeath,
	"must not contain a comma; use the parent place for the larger region": FieldPlaceComma,
	"a place is empty":                                                     FieldPlaceEmpty,
	"name the holder: pick a person or say who in the notes":               FieldHolderRequired,
	"only photos can have face tags":                                       FieldPhotoRequired,
	"a portrait must be a photo":                                           FieldPhotoRequired,
	"is not a face tag on this photo":                                      FieldNotFaceTag,
	"the box must lie within the image":                                    FieldOutsideImage,
	"is not a suggestion":                                                  FieldNotSuggestion,
	"a citation is not attached here":                                      FieldNotAttached,
	"a family event needs two partner roles and no principal":              FieldRolesInvalid,
	"the event needs exactly one principal role":                           FieldRolesInvalid,
	"every role needs its own key":                                         FieldRolesInvalid,
	"exactly one row needs the main role":                                  FieldRowsInvalid,
	"the record needs both partners":                                       FieldRowsInvalid,
	"unknown kind":                                                         FieldUnknown,
	"unknown relation":                                                     FieldUnknown,
	"unknown template":                                                     FieldUnknown,
	"too many holders":                                                     FieldTooMany,
	"too many links":                                                       FieldTooMany,
	"too many names":                                                       FieldTooMany,
}

var (
	reTooLong     = regexp.MustCompile(`\btoo long$`)
	reNotExist    = regexp.MustCompile(`does not exist$`)
	reBetween     = regexp.MustCompile(`^must be between (-?\d+) and (-?\d+)$`)
	reTag         = regexp.MustCompile(`^must be a GEDCOM tag`)
	reMustBe      = regexp.MustCompile(`^must be (.+)$`)
	reNameLen     = regexp.MustCompile(`^(?:give the .+ a name|every role needs a name) of at most (\d+) characters$`)
	reTextLen     = regexp.MustCompile(`^say what .+, in at most (\d+) characters$`)
	reRows        = regexp.MustCompile(`^a record can have at most (\d+) rows$`)
	reCanonical   = regexp.MustCompile(`^item (\d+) is not a canonical GEDCOM date$`)
	reUnknownCol  = regexp.MustCompile(`^unknown or repeated column (.+)$`)
	reUnknownWord = regexp.MustCompile(`^unknown (?:kind|status|role|action|column|name type|participant role|way of getting it:) (.+)$`)
)

// reRowField splits the "<column>: <message>" form in which the transcription
// apply step reports a problem of one row.
var reRowField = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_.]*): (.+)$`)

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// classifyField derives the code and params of a store validation message.
// The store keeps plain English messages; this is the single place that maps
// them to stable codes, and TestStoreMessagesAreClassified pins every message
// the store can produce. Anything not recognised becomes FieldInvalid and the
// client shows the English message.
func classifyField(msg string) FieldError {
	if code, ok := exactFields[msg]; ok {
		return FieldError{Code: code}
	}
	// A row message of a transcription ("<column>: <message>"): classify the
	// part after the column and keep the column as a param. An unknown inner
	// message stays FieldInvalid and the client shows the whole English text.
	if m := reRowField.FindStringSubmatch(msg); m != nil {
		inner := classifyField(m[2])
		if inner.Code == FieldInvalid {
			return inner
		}
		params := map[string]any{"field": m[1]}
		for k, v := range inner.Params {
			params[k] = v
		}
		return FieldError{Code: inner.Code, Params: params}
	}
	switch msg {
	case "must be at least 10 characters":
		return FieldError{Code: FieldPasswordShort, Params: map[string]any{"count": 10}}
	case "must be at most 72 bytes":
		return FieldError{Code: FieldPasswordLong, Params: map[string]any{"count": 72}}
	}
	if m := reBetween.FindStringSubmatch(msg); m != nil {
		return FieldError{Code: FieldOutOfRange, Params: map[string]any{"min": atoi(m[1]), "max": atoi(m[2])}}
	}
	if reTag.MatchString(msg) {
		return FieldError{Code: FieldInvalidTag}
	}
	if m := reNameLen.FindStringSubmatch(msg); m != nil {
		return FieldError{Code: FieldNameLength, Params: map[string]any{"count": atoi(m[1])}}
	}
	if m := reTextLen.FindStringSubmatch(msg); m != nil {
		return FieldError{Code: FieldTextLength, Params: map[string]any{"count": atoi(m[1])}}
	}
	if m := reRows.FindStringSubmatch(msg); m != nil {
		return FieldError{Code: FieldTooManyRows, Params: map[string]any{"count": atoi(m[1])}}
	}
	if m := reCanonical.FindStringSubmatch(msg); m != nil {
		return FieldError{Code: FieldNotCanonical, Params: map[string]any{"item": atoi(m[1])}}
	}
	if strings.HasSuffix(msg, "is listed twice") || strings.HasSuffix(msg, "are listed twice") || strings.Contains(msg, "listed twice") {
		return FieldError{Code: FieldDuplicate}
	}
	if reTooLong.MatchString(msg) {
		return FieldError{Code: FieldTooLong}
	}
	if reNotExist.MatchString(msg) {
		return FieldError{Code: FieldNotFound}
	}
	if m := reUnknownCol.FindStringSubmatch(msg); m != nil {
		return FieldError{Code: FieldUnknownValue, Params: map[string]any{"value": m[1]}}
	}
	if m := reUnknownWord.FindStringSubmatch(msg); m != nil {
		return FieldError{Code: FieldUnknownValue, Params: map[string]any{"value": m[1]}}
	}
	if m := reMustBe.FindStringSubmatch(msg); m != nil {
		return FieldError{Code: FieldInvalidChoice, Params: map[string]any{"options": strings.ReplaceAll(m[1], " or ", ", ")}}
	}
	return FieldError{Code: FieldInvalid}
}
