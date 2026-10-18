package gedcom

import (
	"bytes"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

const sample551 = `0 HEAD
1 SOUR Ancestry.com Family Trees
2 NAME Ancestry.com Member Trees
1 GEDC
2 VERS 5.5.1
2 FORM LINEAGE-LINKED
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Anna /Müller/
2 GIVN Anna
2 SURN Müller
1 SEX F
1 BIRT
2 DATE 12 MAR 1850
2 PLAC Leipzig, Sachsen, Deutschland
1 NOTE Erste Zeile
2 CONT zweite Zeile, die sehr la
2 CONC ng ist
1 EMAIL anna@@example.org
1 FAMS @F1@
0 @F1@ FAM
1 WIFE @I1@
0 TRLR
`

func TestParseStructure(t *testing.T) {
	doc, err := Parse([]byte(sample551))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Version != "5.5.1" || doc.Encoding != "UTF-8" || doc.Source != "Ancestry.com Member Trees" {
		t.Errorf("header: version %q encoding %q source %q", doc.Version, doc.Encoding, doc.Source)
	}
	if len(doc.Records) != 2 || len(doc.Warnings) != 0 {
		t.Fatalf("records %d, warnings %+v", len(doc.Records), doc.Warnings)
	}
	indi := doc.Records[0]
	if indi.Xref != "I1" || indi.Tag != "INDI" {
		t.Errorf("record: %+v", indi)
	}
	if got := indi.First("BIRT").Text("PLAC"); got != "Leipzig, Sachsen, Deutschland" {
		t.Errorf("PLAC %q", got)
	}
	if got := indi.Text("NOTE"); got != "Erste Zeile\nzweite Zeile, die sehr lang ist" {
		t.Errorf("CONT/CONC: %q", got)
	}
	if got := indi.Text("EMAIL"); got != "anna@example.org" {
		t.Errorf("@@ unescape: %q", got)
	}
	if got := indi.First("FAMS").Pointer; got != "F1" {
		t.Errorf("pointer %q", got)
	}
	if indi.First("BIRT").Line != 13 {
		t.Errorf("line numbers: %d", indi.First("BIRT").Line)
	}
}

func TestParseVersion7(t *testing.T) {
	doc, err := Parse([]byte("0 HEAD\n1 GEDC\n2 VERS 7.0\n0 @I1@ INDI\n1 NOTE @@ref and a@@b\n1 FAMC @VOID@\n0 TRLR\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !doc.Is7() {
		t.Fatalf("version %q", doc.Version)
	}
	// GEDCOM 7 only unescapes a leading @@.
	if got := doc.Records[0].Text("NOTE"); got != "@ref and a@@b" {
		t.Errorf("v7 escapes: %q", got)
	}
	if got := doc.Records[0].First("FAMC").Pointer; got != "VOID" {
		t.Errorf("void pointer %q", got)
	}
}

func TestEncodings(t *testing.T) {
	body := "0 @I1@ INDI\n1 NAME Jürgen /Ørsted/\n0 TRLR\n"
	head := func(char string) string { return "0 HEAD\n1 GEDC\n2 VERS 5.5.1\n1 CHAR " + char + "\n" }

	utf16, _, err := transform.Bytes(unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder(), []byte(head("UNICODE")+body))
	if err != nil {
		t.Fatal(err)
	}
	ansi, _, err := transform.Bytes(charmap.Windows1252.NewEncoder(), []byte(head("ANSI")+body))
	if err != nil {
		t.Fatal(err)
	}
	// ANSEL: the diaeresis (0xE8) comes before the u; Ø is 0xA2.
	ansel := append([]byte(head("ANSEL")+"0 @I1@ INDI\n1 NAME J"), 0xE8, 'u')
	ansel = append(ansel, []byte("rgen /")...)
	ansel = append(ansel, 0xA2)
	ansel = append(ansel, []byte("rsted/\n0 TRLR\n")...)
	// "UTF-8" in the header but saved as Windows-1252, a common mistake.
	mislabeled, _, _ := transform.Bytes(charmap.Windows1252.NewEncoder(), []byte(head("UTF-8")+body))

	tests := []struct {
		name, encoding string
		data           []byte
		warn           bool
	}{
		{"utf-8 with BOM", "UTF-8", append([]byte{0xEF, 0xBB, 0xBF}, head("UTF-8")+body...), false},
		{"utf-16", "UTF-16", utf16, false},
		{"ansi", "ANSI", ansi, false},
		{"ansel", "ANSEL", ansel, false},
		{"mislabeled", "ANSI", mislabeled, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := Parse(tt.data)
			if err != nil {
				t.Fatal(err)
			}
			if doc.Encoding != tt.encoding {
				t.Errorf("encoding %q, want %q", doc.Encoding, tt.encoding)
			}
			if got := doc.Records[0].Text("NAME"); got != "Jürgen /Ørsted/" {
				t.Errorf("name %q", got)
			}
			if (len(doc.Warnings) > 0) != tt.warn {
				t.Errorf("warnings %+v", doc.Warnings)
			}
		})
	}
}

func TestMalformedInput(t *testing.T) {
	if _, err := Parse([]byte("<html><body>not a gedcom</body></html>")); err == nil {
		t.Error("HTML must be rejected")
	}
	if _, err := Parse([]byte("   \n\n")); err == nil {
		t.Error("an empty file must be rejected")
	}
	doc, err := Parse([]byte("0 HEAD\r\n1 GEDC\r2 VERS 5.5.1\n0 @I1@ INDI\n1 NAME A /B/\n3 DATE 1850\nthis is junk\n2 SURN B\n0 TRLR"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Version != "5.5.1" {
		t.Errorf("mixed line endings: version %q", doc.Version)
	}
	if len(doc.Warnings) != 2 {
		t.Errorf("want warnings for the level jump and the junk line: %+v", doc.Warnings)
	}
	name := doc.Records[0].First("NAME")
	if name == nil || name.Text("DATE") != "1850" || name.Text("SURN") != "B" {
		t.Errorf("recovered structure: %+v", name)
	}
}

func TestParseTolerantSpacing(t *testing.T) {
	// Built from interpreted strings: the trailing spaces and tabs matter.
	src := strings.Join([]string{
		"0 HEAD",
		"1 GEDC",
		"2 VERS 5.5.1",
		"0 @I1@  INDI",
		"1 NAME Anna /Weber/  ",
		"1 FAMC @F1@ ",
		"1 FAMS @F2@\t",
		"1  FAMC  @F3@",
		"1 NOTE abc ",
		"2 CONC def ",
		"1 BIRT ",
		"0 @I2@INDI",
		"0 @F1@   FAM ",
		"1 CHIL @I1@",
		"0 TRLR",
	}, "\n")
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Records) != 2 {
		t.Fatalf("want records I1 and F1, got %d", len(doc.Records))
	}
	if len(doc.Warnings) != 1 || !strings.Contains(doc.Warnings[0].Message, "unreadable line") {
		t.Errorf("want one unreadable-line warning: %+v", doc.Warnings)
	}
	indi, fam := doc.Records[0], doc.Records[1]
	if indi.Xref != "I1" || indi.Tag != "INDI" {
		t.Errorf("record 0: %q %q", indi.Xref, indi.Tag)
	}
	if fam.Xref != "F1" || fam.Tag != "FAM" {
		t.Errorf("record 1: %q %q", fam.Xref, fam.Tag)
	}
	if c := fam.First("CHIL"); c == nil || c.Pointer != "I1" {
		t.Errorf("CHIL: %+v", c)
	}
	famc := indi.All("FAMC")
	if len(famc) != 2 || famc[0].Pointer != "F1" || famc[1].Pointer != "F3" || famc[0].Value != "" || famc[1].Value != "" {
		t.Errorf("FAMC: %+v", famc)
	}
	if f := indi.First("FAMS"); f == nil || f.Pointer != "F2" {
		t.Errorf("FAMS: %+v", f)
	}
	if got := indi.Text("NAME"); got != "Anna /Weber/" {
		t.Errorf("NAME %q", got)
	}
	if got := indi.Text("NOTE"); got != "abc def" {
		t.Errorf("NOTE %q", got)
	}
	if b := indi.First("BIRT"); b == nil || b.Value != "" {
		t.Errorf("BIRT: %+v", b)
	}
}

func TestParseNeverPanics(t *testing.T) {
	data := []byte(sample551)
	for i := range len(data) {
		_, _ = Parse(data[:i])
		bad := bytes.Clone(data)
		bad[i] = "@ \n0x\xff"[i%5]
		_, _ = Parse(bad)
	}
	_, _ = Parse([]byte(strings.Repeat("9", 100) + " X"))
}
