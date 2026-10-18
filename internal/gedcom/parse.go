// Package gedcom reads GEDCOM 5.5, 5.5.1 and 7.0 files into a tree of
// records, independent of what GoTree does with them.
package gedcom

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// Kinds of ParseError: the reasons a file is refused. Callers use them to give
// stable machine codes; the error text stays English.
const (
	ErrNotGedcom = "not_gedcom" // the first lines are not GEDCOM lines; Line is set
	ErrEmpty     = "empty"      // the file has no lines
)

// ParseError is a file that is not GEDCOM at all.
type ParseError struct {
	Kind string
	Line int
	Err  error
	msg  string
}

func (e *ParseError) Error() string { return e.msg }
func (e *ParseError) Unwrap() error { return e.Err }

// Node is one line of a GEDCOM file with the lines nested below it.
type Node struct {
	Level int
	// Xref is the record identifier without the @ signs, e.g. "I1".
	Xref string
	Tag  string
	// Value is the line value with CONC/CONT continuation lines joined,
	// trailing spaces removed and @@ unescaped. For pointers it is empty and
	// Pointer is set.
	Value string
	// Pointer is the referenced xref without @ signs, e.g. "F2", or
	// "VOID" for a GEDCOM 7 void pointer.
	Pointer  string
	Line     int
	Children []*Node
}

// First returns the first child with the tag, or nil.
func (n *Node) First(tag string) *Node {
	for _, c := range n.Children {
		if c.Tag == tag {
			return c
		}
	}
	return nil
}

// All returns the children with the tag.
func (n *Node) All(tag string) []*Node {
	var out []*Node
	for _, c := range n.Children {
		if c.Tag == tag {
			out = append(out, c)
		}
	}
	return out
}

// Text returns the value of the first child with the tag, or "".
func (n *Node) Text(tag string) string {
	if c := n.First(tag); c != nil {
		return c.Value
	}
	return ""
}

// Warning is a problem found while reading that did not stop it.
type Warning struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// Document is a parsed GEDCOM file.
type Document struct {
	// Version is HEAD.GEDC.VERS, e.g. "5.5.1" or "7.0", or "" if missing.
	Version string
	// Encoding is the character set the file was decoded from.
	Encoding string
	// Source is HEAD.SOUR: the program that wrote the file.
	Source string
	Head   *Node
	// Records are the level-0 records other than HEAD and TRLR.
	Records  []*Node
	Warnings []Warning

	byXref map[string]*Node
}

// RecordByXref finds a level-0 record by its identifier.
func (d *Document) RecordByXref(xref string) (*Node, bool) {
	if d.byXref == nil {
		d.byXref = make(map[string]*Node, len(d.Records))
		for _, r := range d.Records {
			if r.Xref != "" {
				d.byXref[r.Xref] = r
			}
		}
	}
	n, ok := d.byXref[xref]
	return n, ok
}

// Is7 reports whether the file declares GEDCOM 7.
func (d *Document) Is7() bool {
	return strings.HasPrefix(d.Version, "7")
}

func (d *Document) warn(line int, format string, args ...any) {
	if len(d.Warnings) < 1000 {
		d.Warnings = append(d.Warnings, Warning{Line: line, Message: fmt.Sprintf(format, args...)})
	}
}

var charDecl = regexp.MustCompile(`(?m)^\s*1\s+CHAR\s+(\S+)`)

// Parse reads a GEDCOM file. Only input that is not GEDCOM at all is an
// error; problems inside the file become warnings.
func Parse(data []byte) (*Document, error) {
	doc := &Document{}
	text, err := decode(data, doc)
	if err != nil {
		return nil, err
	}
	if err := doc.parseLines(text); err != nil {
		return nil, err
	}
	return doc, nil
}

func decode(data []byte, doc *Document) (string, error) {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		doc.Encoding = "UTF-8"
		return string(data[3:]), nil
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}), bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		doc.Encoding = "UTF-16"
		out, _, err := transform.Bytes(unicode.UTF16(unicode.BigEndian, unicode.UseBOM).NewDecoder(), data)
		if err != nil {
			return "", fmt.Errorf("decode UTF-16: %w", err)
		}
		return string(out), nil
	}

	declared := "UTF-8"
	head := data[:min(len(data), 4096)]
	if m := charDecl.FindSubmatch(head); m != nil {
		declared = strings.ToUpper(string(m[1]))
	}
	switch declared {
	case "ANSEL":
		doc.Encoding = "ANSEL"
		return decodeANSEL(data, doc), nil
	case "ANSI", "WINDOWS-1252", "CP1252", "IBMPC", "IBM", "MACINTOSH":
		doc.Encoding = "ANSI"
		out, _, err := transform.Bytes(charmap.Windows1252.NewDecoder(), data)
		return string(out), err
	case "UNICODE":
		// UNICODE means UTF-16, which needs a BOM; without one the file is
		// in practice UTF-8.
		fallthrough
	default:
		doc.Encoding = "UTF-8"
		if utf8.Valid(data) {
			return string(data), nil
		}
		// Many programs write "UTF-8" in the header and then save in the
		// Windows code page; reading it as such recovers umlauts.
		doc.warn(0, "the file says %s but is not valid UTF-8; read as Windows-1252", declared)
		doc.Encoding = "ANSI"
		out, _, err := transform.Bytes(charmap.Windows1252.NewDecoder(), data)
		return string(out), err
	}
}

func (d *Document) parseLines(text string) error {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")

	var stack []*Node
	var roots []*Node
	parsed := 0
	for i, raw := range lines {
		lineNo := i + 1
		line := strings.TrimLeft(raw, " \t")
		if strings.TrimSpace(line) == "" {
			continue
		}
		n, ok := parseLine(line)
		if !ok {
			if parsed == 0 && i < 3 {
				return &ParseError{Kind: ErrNotGedcom, Line: lineNo, msg: fmt.Sprintf("this does not look like a GEDCOM file (line %d)", lineNo)}
			}
			d.warn(lineNo, "unreadable line skipped: %q", truncate(line, 80))
			continue
		}
		n.Line = lineNo
		parsed++

		for len(stack) > 0 && stack[len(stack)-1].Level >= n.Level {
			stack = stack[:len(stack)-1]
		}
		if n.Level == 0 {
			roots = append(roots, n)
		} else if len(stack) == 0 {
			d.warn(lineNo, "line at level %d without a record above it skipped", n.Level)
			continue
		} else {
			parent := stack[len(stack)-1]
			if n.Level > parent.Level+1 {
				d.warn(lineNo, "level jumps from %d to %d; attached to the line above", parent.Level, n.Level)
				n.Level = parent.Level + 1
			}
			parent.Children = append(parent.Children, n)
		}
		stack = append(stack, n)
	}
	if parsed == 0 {
		return &ParseError{Kind: ErrEmpty, msg: "the file is empty"}
	}

	for _, r := range roots {
		if r.Tag == "HEAD" && d.Head == nil {
			d.Head = r
			joinContinuations(r, false)
		}
	}
	if d.Head != nil {
		if g := d.Head.First("GEDC"); g != nil {
			d.Version = g.Text("VERS")
		}
	}
	for _, r := range roots {
		switch r.Tag {
		case "HEAD", "TRLR":
		default:
			joinContinuations(r, d.Is7())
			d.Records = append(d.Records, r)
		}
	}
	if d.Head == nil {
		d.warn(1, "the file has no header")
	} else {
		if s := d.Head.First("SOUR"); s != nil {
			d.Source = s.Value
			if name := s.Text("NAME"); name != "" {
				d.Source = name
			}
		}
	}
	return nil
}

// parseLine splits "level [@xref@] TAG [value]"; extra spaces between the
// parts are tolerated.
func parseLine(line string) (*Node, bool) {
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == 0 || i > 2 || i >= len(line) || line[i] != ' ' {
		return nil, false
	}
	level := 0
	for _, c := range line[:i] {
		level = level*10 + int(c-'0')
	}
	rest := strings.TrimLeft(line[i+1:], " ")
	n := &Node{Level: level}

	if strings.HasPrefix(rest, "@") {
		end := strings.Index(rest[1:], "@")
		if end < 0 || end+2 >= len(rest) || rest[end+2] != ' ' {
			return nil, false
		}
		n.Xref = rest[1 : end+1]
		rest = strings.TrimLeft(rest[end+2:], " ")
	}

	tag, value, _ := strings.Cut(rest, " ")
	if tag == "" {
		return nil, false
	}
	for _, c := range tag {
		ok := c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
		if !ok {
			return nil, false
		}
	}
	n.Tag = strings.ToUpper(tag)

	pv := strings.TrimSpace(value)
	if len(pv) > 2 && pv[0] == '@' && pv[len(pv)-1] == '@' && !strings.Contains(pv[1:len(pv)-1], "@") && pv[1] != '#' {
		n.Pointer = pv[1 : len(pv)-1]
	} else {
		n.Value = value
	}
	return n, true
}

// joinContinuations folds CONC and CONT lines into their parent's value
// and unescapes @@, recursively: every @@ in 5.5.1, only a leading one in
// GEDCOM 7.
func joinContinuations(n *Node, v7 bool) {
	kept := n.Children[:0]
	var b strings.Builder
	b.WriteString(n.Value)
	joined := false
	for _, c := range n.Children {
		switch c.Tag {
		case "CONT":
			b.WriteByte('\n')
			b.WriteString(c.Value)
			joined = true
		case "CONC":
			b.WriteString(c.Value)
			joined = true
		default:
			kept = append(kept, c)
		}
	}
	n.Children = kept
	if joined {
		n.Value = b.String()
	}
	n.Value = strings.TrimRight(n.Value, " \t")
	if v7 {
		if strings.HasPrefix(n.Value, "@@") {
			n.Value = n.Value[1:]
		}
	} else {
		n.Value = strings.ReplaceAll(n.Value, "@@", "@")
	}
	for _, c := range n.Children {
		joinContinuations(c, v7)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
