package api

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type codedError struct {
	Error      string                `json:"error"`
	Code       string                `json:"code"`
	Params     map[string]any        `json:"params"`
	Fields     map[string]string     `json:"fields"`
	FieldCodes map[string]FieldError `json:"fieldCodes"`
}

func callError(t *testing.T, c *client, method, path string, body any, wantStatus int) codedError {
	t.Helper()
	status, raw := c.do(method, path, body)
	if status != wantStatus {
		t.Fatalf("%s %s: status %d, want %d: %s", method, path, status, wantStatus, raw)
	}
	var e codedError
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatalf("%s %s: %v: %s", method, path, err, raw)
	}
	if e.Error == "" {
		t.Errorf("%s %s: the English message must stay in the response: %s", method, path, raw)
	}
	return e
}

func TestErrorResponsesCarryCodes(t *testing.T) {
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)

	t.Run("validation", func(t *testing.T) {
		e := callError(t, c, "POST", "/api/persons", map[string]string{"sex": "Z", "givenNames": strings.Repeat("x", 300)}, http.StatusUnprocessableEntity)
		if e.Code != CodeInvalidInput {
			t.Errorf("code %q", e.Code)
		}
		if got := e.FieldCodes["sex"].Code; got != FieldInvalidChoice {
			t.Errorf("sex code %q (%+v)", got, e.FieldCodes)
		}
		if got := e.FieldCodes["givenNames"].Code; got != FieldTooLong {
			t.Errorf("givenNames code %q (%+v)", got, e.FieldCodes)
		}
		if e.Fields["sex"] == "" {
			t.Error("the English field message must stay")
		}
	})

	t.Run("not found", func(t *testing.T) {
		e := callError(t, c, "GET", "/api/persons/99999", nil, http.StatusNotFound)
		if e.Code != CodeNotFound {
			t.Errorf("code %q", e.Code)
		}
	})

	t.Run("too large with params", func(t *testing.T) {
		status, raw := c.upload("/api/media", "big.jpg", append(jpegBytes(t, 8, 8), make([]byte, 2<<20)...))
		var e codedError
		_ = json.Unmarshal(raw, &e)
		if status != http.StatusRequestEntityTooLarge || e.Code != CodeFileTooLarge || e.Params["limitMB"] != float64(1) {
			t.Errorf("%d %s", status, raw)
		}
	})

	t.Run("conflict", func(t *testing.T) {
		other := newClient(t, ts)
		e := callError(t, other, "POST", "/api/auth/setup", map[string]string{"username": "evil", "password": testPassword}, http.StatusConflict)
		if e.Code != CodeSetupDone {
			t.Errorf("code %q", e.Code)
		}
	})

	t.Run("unauthorized", func(t *testing.T) {
		e := callError(t, newClient(t, ts), "GET", "/api/persons", nil, http.StatusUnauthorized)
		if e.Code != CodeNotLoggedIn {
			t.Errorf("code %q", e.Code)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		if _, err := testServers[ts].Store.DB.Exec(`UPDATE tree_members SET role = 'viewer'`); err != nil {
			t.Fatal(err)
		}
		e := callError(t, c, "POST", "/api/persons", map[string]string{"givenNames": "After"}, http.StatusForbidden)
		if e.Code != CodeViewOnly {
			t.Errorf("code %q", e.Code)
		}
	})
}

// The codes the server defines and the apiError keys in en.json must be the
// same set: a new code cannot ship without text, and a stale text is removed.
func TestCodesMatchTranslations(t *testing.T) {
	raw, err := os.ReadFile("../../web/src/locales/en.json")
	if err != nil {
		t.Fatal(err)
	}
	var locale struct {
		APIError map[string]json.RawMessage `json:"apiError"`
	}
	if err := json.Unmarshal(raw, &locale); err != nil {
		t.Fatal(err)
	}
	var field map[string]string
	if err := json.Unmarshal(locale.APIError["field"], &field); err != nil {
		t.Fatal(err)
	}

	haveRequest, haveField := map[string]bool{}, map[string]bool{}
	for k := range locale.APIError {
		if k != "field" && k != "option" {
			haveRequest[k] = true
		}
	}
	for k := range field {
		for _, suffix := range []string{"_one", "_other"} {
			k = strings.TrimSuffix(k, suffix)
		}
		haveField[k] = true
	}

	file, err := parser.ParseFile(token.NewFileSet(), "errcodes.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	wantRequest, wantField := map[string]bool{}, map[string]bool{}
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			code, _ := strconv.Unquote(vs.Values[0].(*ast.BasicLit).Value)
			switch {
			case strings.HasPrefix(vs.Names[0].Name, "Code"):
				wantRequest[code] = true
			case strings.HasPrefix(vs.Names[0].Name, "Field") && code != FieldInvalid:
				// FieldInvalid is the catch-all: the client shows the server's message.
				wantField[code] = true
			}
		}
	}
	if !reflect.DeepEqual(wantRequest, haveRequest) {
		t.Errorf("request codes and apiError keys differ:\n codes: %v\n keys:  %v", sortedKeys(wantRequest), sortedKeys(haveRequest))
	}
	if !reflect.DeepEqual(wantField, haveField) {
		t.Errorf("field codes and apiError.field keys differ:\n codes: %v\n keys:  %v", sortedKeys(wantField), sortedKeys(haveField))
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestClassifyField(t *testing.T) {
	for msg, want := range map[string]FieldError{
		"is required":                               {Code: FieldRequired},
		"is too long":                               {Code: FieldTooLong},
		"does not exist":                            {Code: FieldNotFound},
		"a linked person does not exist":            {Code: FieldNotFound},
		"must be a date":                            {Code: FieldInvalidDate},
		"must be between -90 and 90":                {Code: FieldOutOfRange, Params: map[string]any{"min": -90, "max": 90}},
		"must be M, F, U or X":                      {Code: FieldInvalidChoice, Params: map[string]any{"options": "M, F, U, X"}},
		"unknown role chief":                        {Code: FieldUnknownValue, Params: map[string]any{"value": "chief"}},
		"say why the fact is disputed or disproven": {Code: FieldReasonRequired},
		"something nobody anticipated":              {Code: FieldInvalid},
	} {
		if got := classifyField(msg); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %+v, want %+v", msg, got, want)
		}
	}
}
