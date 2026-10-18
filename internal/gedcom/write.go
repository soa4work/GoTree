package gedcom

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

// maxLine551 is the longest line GEDCOM 5.5.1 allows; longer values are
// continued with CONC.
const maxLine551 = 255

// WriteOptions selects the GEDCOM version to write.
type WriteOptions struct {
	Version7 bool
}

// Write serializes level-0 records (HEAD first, TRLR is added).
// Levels are taken from the nesting, not from Node.Level.
func Write(w io.Writer, records []*Node, opt WriteOptions) error {
	bw := bufio.NewWriter(w)
	for _, r := range records {
		if err := writeNode(bw, r, 0, opt); err != nil {
			return err
		}
	}
	if _, err := bw.WriteString("0 TRLR\n"); err != nil {
		return err
	}
	return bw.Flush()
}

func writeNode(w *bufio.Writer, n *Node, level int, opt WriteOptions) error {
	prefix := fmt.Sprintf("%d ", level)
	if n.Xref != "" {
		prefix += "@" + n.Xref + "@ "
	}
	prefix += n.Tag

	value := n.Value
	if n.Pointer != "" {
		value = "@" + n.Pointer + "@"
	}

	for i, line := range strings.Split(value, "\n") {
		if n.Pointer == "" {
			// Every line is a line of its own, so every line is escaped.
			line = escape(line, opt.Version7)
		}
		head := prefix
		if i > 0 {
			head = fmt.Sprintf("%d CONT", level+1)
		}
		chunks := []string{line}
		if !opt.Version7 {
			// CONC lines may have a longer head than a short tag.
			conc := len(fmt.Sprintf("%d CONC", level+1))
			chunks = splitForCONC(line, maxLine551-max(len(head), conc)-1)
		}
		for j, chunk := range chunks {
			h := head
			if j > 0 {
				h = fmt.Sprintf("%d CONC", level+1)
			}
			if chunk != "" {
				h += " " + chunk
			}
			if _, err := w.WriteString(h + "\n"); err != nil {
				return err
			}
		}
	}
	for _, c := range n.Children {
		if err := writeNode(w, c, level+1, opt); err != nil {
			return err
		}
	}
	return nil
}

// calendarEscape matches 5.5.1 escape sequences such as @#DJULIAN@,
// which stay as they are.
var calendarEscape = regexp.MustCompile(`@#D[A-Z][A-Z ]*@`)

// escape doubles @ the way each version requires: every @ in 5.5.1 text
// except in escape sequences, only a leading @ in GEDCOM 7.
func escape(v string, v7 bool) string {
	if v7 {
		if strings.HasPrefix(v, "@") {
			return "@" + v
		}
		return v
	}
	var b strings.Builder
	last := 0
	for _, loc := range calendarEscape.FindAllStringIndex(v, -1) {
		b.WriteString(strings.ReplaceAll(v[last:loc[0]], "@", "@@"))
		b.WriteString(v[loc[0]:loc[1]])
		last = loc[1]
	}
	b.WriteString(strings.ReplaceAll(v[last:], "@", "@@"))
	return b.String()
}

// splitForCONC cuts a line into pieces of at most max bytes without
// splitting a UTF-8 character or ending a piece on a space, which some
// readers trim.
func splitForCONC(s string, max int) []string {
	if max < 20 {
		max = 20
	}
	var out []string
	for len(s) > max {
		cut := max
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		for cut > 1 && (s[cut-1] == ' ' || s[cut] == ' ') {
			cut--
		}
		if cut <= 1 {
			cut = max
			for cut > 0 && !utf8.RuneStart(s[cut]) {
				cut--
			}
		}
		// Never cut an escaped "@@" in two: a piece starting with a lone @
		// could read as a pointer.
		if s[cut] == '@' {
			run := 0
			for run < cut && s[cut-1-run] == '@' {
				run++
			}
			if run%2 == 1 && cut > 1 && s[cut-2] != ' ' {
				cut--
			}
		}
		out = append(out, s[:cut])
		s = s[cut:]
	}
	return append(out, s)
}
