package gedcom

import (
	"bytes"
	"strings"
	"testing"
)

func write(t *testing.T, records []*Node, v7 bool) string {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(&buf, records, WriteOptions{Version7: v7}); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestWriteBasics(t *testing.T) {
	recs := []*Node{
		{Tag: "HEAD", Children: []*Node{{Tag: "GEDC", Children: []*Node{{Tag: "VERS", Value: "5.5.1"}}}}},
		{Xref: "I1", Tag: "INDI", Children: []*Node{
			{Tag: "NAME", Value: "Anna /Müller/"},
			{Tag: "EMAIL", Value: "anna@example.org"},
			{Tag: "NOTE", Value: "first line\nsecond line"},
			{Tag: "FAMS", Pointer: "F1"},
			{Tag: "DEAT", Value: "Y"},
		}},
	}
	got := write(t, recs, false)
	want := `0 HEAD
1 GEDC
2 VERS 5.5.1
0 @I1@ INDI
1 NAME Anna /Müller/
1 EMAIL anna@@example.org
1 NOTE first line
2 CONT second line
1 FAMS @F1@
1 DEAT Y
0 TRLR
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}

	v7 := write(t, []*Node{{Xref: "I1", Tag: "INDI", Children: []*Node{
		{Tag: "EMAIL", Value: "anna@example.org"},
		{Tag: "NOTE", Value: "@start"},
	}}}, true)
	if !strings.Contains(v7, "1 EMAIL anna@example.org\n") || !strings.Contains(v7, "1 NOTE @@start\n") {
		t.Errorf("GEDCOM 7 escaping:\n%s", v7)
	}
}

func TestWriteLongLines(t *testing.T) {
	long := strings.Repeat("Ä wörd ", 100)
	recs := []*Node{{Xref: "N1", Tag: "NOTE", Value: long + "\n" + "end"}}

	out := write(t, recs, false)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if len(line) > maxLine551 {
			t.Errorf("line of %d bytes: %q", len(line), line[:40])
		}
	}
	if !strings.Contains(out, "1 CONC ") {
		t.Error("5.5.1 must continue long lines with CONC")
	}

	// Reading the output back restores the text exactly.
	for _, v7 := range []bool{false, true} {
		head := `0 HEAD
1 GEDC
2 VERS 5.5.1
`
		if v7 {
			head = strings.Replace(head, "5.5.1", "7.0", 1)
		}
		doc, err := Parse([]byte(head + write(t, recs, v7)))
		if err != nil {
			t.Fatal(err)
		}
		if got := doc.Records[0].Value; got != long+"\nend" {
			t.Errorf("v7=%v: round trip changed the text (%d vs %d bytes)", v7, len(got), len(long)+4)
		}
	}
	if strings.Contains(write(t, recs, true), "CONC") {
		t.Error("GEDCOM 7 has no CONC")
	}
}

func TestSplitForCONC(t *testing.T) {
	for _, s := range []string{strings.Repeat("ü", 300), strings.Repeat("a ", 200), strings.Repeat("x", 1000)} {
		parts := splitForCONC(s, 100)
		if strings.Join(parts, "") != s {
			t.Fatal("pieces do not add up")
		}
		for _, p := range parts {
			if len(p) > 100 || !strings.HasPrefix(p, string([]rune(p)[0])) {
				t.Errorf("bad piece %q", p)
			}
		}
	}
}

func TestEscapeKeepsCalendars(t *testing.T) {
	if got := escape("@#DJULIAN@ 4 OCT 1582 by me@home", false); got != "@#DJULIAN@ 4 OCT 1582 by me@@home" {
		t.Errorf("got %q", got)
	}
}

func TestWriteEscapesEveryLine(t *testing.T) {
	recs := []*Node{{Xref: "N1", Tag: "NOTE", Value: "a\n@b\nc@d"}}
	out := write(t, recs, true)
	if !strings.Contains(out, "1 CONT @@b\n") || !strings.Contains(out, "1 CONT c@d\n") {
		t.Errorf("7.0 output:\n%s", out)
	}
	roundTrip := func(value string, v7 bool) (string, string) {
		out := write(t, []*Node{{Xref: "N1", Tag: "NOTE", Value: value}}, v7)
		ver := "5.5.1"
		if v7 {
			ver = "7.0"
		}
		doc, err := Parse([]byte("0 HEAD\n1 GEDC\n2 VERS " + ver + "\n" + out))
		if err != nil {
			t.Fatal(err)
		}
		return out, doc.Records[0].Value
	}
	for _, v7 := range []bool{false, true} {
		if _, got := roundTrip("a\n@b\nc@d\n@I1@\n@", v7); got != "a\n@b\nc@d\n@I1@\n@" {
			t.Errorf("v7=%v: round trip %q", v7, got)
		}
	}

	long := strings.Repeat("@", 600)
	out, got := roundTrip(long, false)
	if got != long {
		t.Errorf("long @ run changed: %d characters", len(got))
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if len(line) > maxLine551 {
			t.Errorf("line of %d bytes", len(line))
		}
		if v, ok := strings.CutPrefix(line, "1 CONC "); ok && strings.HasPrefix(v, "@") && !strings.HasPrefix(v, "@@") {
			t.Errorf("CONC starts with a lone @: %q", line)
		}
	}
}
