package store

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/praetorianer777/gotree/internal/gedcom"
)

// reportLines joins the English messages of report lines.
func reportLines(ws []gedcom.Warning) string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = w.Message
	}
	return strings.Join(out, "\n")
}

func findLine(ws []gedcom.Warning, code string) *gedcom.Warning {
	for i := range ws {
		if ws[i].Code == code {
			return &ws[i]
		}
	}
	return nil
}

func TestReportLinesCarryCodes(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	report, err := s.ImportGEDCOM(ctx, a, parseFixture(t, "broken.ged"), ImportIntoEmpty)
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, c := range ReportCodes {
		known[c] = true
	}
	for _, list := range [][]gedcom.Warning{report.BrokenReferences, report.InvalidDateExamples, report.ChildrenFromFAMCExamples} {
		for _, w := range list {
			if !known[w.Code] || w.Message == "" || w.Line <= 0 {
				t.Errorf("report line %+v needs a known code, a message and a line", w)
			}
		}
	}
	if w := findLine(report.BrokenReferences, ReportBrokenReference); w == nil || w.Params["what"] == "" || w.Params["xref"] == "" {
		t.Errorf("no broken_reference with params in %+v", report.BrokenReferences)
	}
	if w := findLine(report.BrokenReferences, ReportPartnerAndChild); w == nil || w.Params["xref"] == "" || w.Params["family"] == "" {
		t.Errorf("no partner_and_child with params in %+v", report.BrokenReferences)
	}
	if w := findLine(report.InvalidDateExamples, ReportInvalidDate); w == nil || w.Params["date"] != w.Message {
		t.Errorf("invalid dates: %+v", report.InvalidDateExamples)
	}
}

func TestBrokenFamilyReferenceParams(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	src := strings.Join([]string{
		"0 HEAD", "1 GEDC", "2 VERS 5.5.1", "1 CHAR UTF-8",
		"0 @I1@ INDI", "1 NAME Anna /Weber/", "1 FAMC @F9@",
		"0 @F1@ FAM", "1 HUSB @I1@", "1 CHIL @I8@",
		"0 TRLR",
	}, "\n")
	doc, err := gedcom.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	report, err := s.ImportGEDCOM(ctx, a, doc, ImportIntoEmpty)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, w := range report.BrokenReferences {
		if w.Code == ReportBrokenReference {
			got[w.Params["xref"].(string)] = w.Params["what"].(string)
		}
	}
	if got["I8"] != RefChild {
		t.Errorf("broken references: %+v", report.BrokenReferences)
	}
}

func TestEveryDroppedTagHasReasonCode(t *testing.T) {
	known := map[string]bool{}
	for _, c := range DropReasonCodes {
		known[c] = true
	}
	for _, name := range []string{"ancestry-551.ged", "broken.ged", "gedcom7.ged"} {
		s := newStore(t)
		a := setup(t, s)
		report, err := s.ImportGEDCOM(ctx, a, parseFixture(t, name), ImportIntoEmpty)
		if err != nil {
			t.Fatal(err)
		}
		for _, tg := range report.Tags {
			if tg.Outcome == TagDropped && (!known[tg.ReasonCode] || tg.Reason == "") {
				t.Errorf("%s: dropped %s has reason %q, code %q", name, tg.Path, tg.Reason, tg.ReasonCode)
			}
			if tg.Outcome != TagDropped && (tg.Reason != "" || tg.ReasonCode != "") {
				t.Errorf("%s: %s %s carries a reason", name, tg.Outcome, tg.Path)
			}
		}
	}
	for code := range dropReasonText {
		if !known[code] {
			t.Errorf("dropReasonText has %q, which is not in DropReasonCodes", code)
		}
	}
	if len(dropReasonText) != len(DropReasonCodes) {
		t.Errorf("%d texts for %d codes", len(dropReasonText), len(DropReasonCodes))
	}
	for _, code := range droppedTags {
		if !known[code] {
			t.Errorf("droppedTags uses unknown code %q", code)
		}
	}
}

// TestReportCodesMatchTranslations keeps the locale files and the codes the
// server sends in step, like the api package does for error codes.
func TestReportCodesMatchTranslations(t *testing.T) {
	raw, err := os.ReadFile("../../web/src/locales/en.json")
	if err != nil {
		t.Fatal(err)
	}
	var locale struct {
		Transfer struct {
			Warning    map[string]string `json:"warning"`
			DropReason map[string]string `json:"dropReason"`
			RefKind    map[string]string `json:"refKind"`
		} `json:"transfer"`
	}
	if err := json.Unmarshal(raw, &locale); err != nil {
		t.Fatal(err)
	}
	keys := func(m map[string]string) []string {
		seen := map[string]bool{}
		for k := range m {
			for _, suffix := range []string{"_one", "_other"} {
				k = strings.TrimSuffix(k, suffix)
			}
			seen[k] = true
		}
		out := make([]string, 0, len(seen))
		for k := range seen {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}
	sorted := func(in ...[]string) []string {
		var out []string
		for _, l := range in {
			out = append(out, l...)
		}
		sort.Strings(out)
		return out
	}
	if got, want := keys(locale.Transfer.Warning), sorted(gedcom.WarningCodes, ReportCodes); !reflect.DeepEqual(got, want) {
		t.Errorf("transfer.warning keys %v, want %v", got, want)
	}
	if got, want := keys(locale.Transfer.DropReason), sorted(DropReasonCodes); !reflect.DeepEqual(got, want) {
		t.Errorf("transfer.dropReason keys %v, want %v", got, want)
	}
	if got, want := keys(locale.Transfer.RefKind), sorted(RefKinds); !reflect.DeepEqual(got, want) {
		t.Errorf("transfer.refKind keys %v, want %v", got, want)
	}
	for what := range refKindText {
		found := false
		for _, k := range RefKinds {
			found = found || k == what
		}
		if !found {
			t.Errorf("refKindText has %q, which is not in RefKinds", what)
		}
	}
}
