package media

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// cleanVersion names the stripping rules in the file names under Root/clean.
// Bump it when the rules change, so copies made by older rules are not served.
const cleanVersion = "v1"

// maxChunk bounds the metadata blocks that are read whole (PNG and WebP
// chunks); a larger one is not trusted and the file is refused.
const maxChunk = 16 << 20

var errMetaTooLarge = errors.New("metadata block too large to inspect")

// WithoutLocation returns the path of the file to hand out for the stored
// original: a copy with the GPS position blanked when the original has one
// (JPEG, PNG and WebP, in EXIF and XMP), else the original itself. The
// original is never changed, so backups restore exactly what was uploaded.
// The copy has the same size and the same image data, so orientation and
// capture time survive. An error means no safe file exists; callers must not
// fall back to the original.
func (f Files) WithoutLocation(sha, mime string) (string, error) {
	orig, err := f.Path(sha)
	if err != nil {
		return "", err
	}
	var strip func(*os.File) (bool, error)
	switch mime {
	case "image/jpeg":
		strip = stripJPEG
	case "image/png":
		strip = stripPNG
	case "image/webp":
		strip = stripWebP
	default:
		return orig, nil
	}

	dir := filepath.Join(f.Root, "clean", sha[:2])
	clean := filepath.Join(dir, sha+"-"+cleanVersion)
	same := clean + ".same"
	if _, err := os.Stat(clean); err == nil {
		return clean, nil
	}
	if _, err := os.Stat(same); err == nil {
		return orig, nil
	}

	tmpDir := filepath.Join(f.Root, "tmp")
	if err := os.MkdirAll(tmpDir, 0o750); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(tmpDir, "clean-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name()) // a no-op after the rename
	defer tmp.Close()
	src, err := os.Open(orig)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(tmp, src)
	src.Close()
	if err != nil {
		return "", err
	}
	changed, err := strip(tmp)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	if !changed {
		// Remember that this file has nothing to remove, so it is not copied again.
		m, err := os.OpenFile(same, os.O_CREATE|os.O_WRONLY, 0o640)
		if err != nil {
			return "", err
		}
		return orig, m.Close()
	}
	if err := tmp.Sync(); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	// Concurrent requests build identical bytes; the last rename wins.
	if err := os.Rename(tmp.Name(), clean); err != nil {
		return "", err
	}
	return clean, nil
}

// blankTIFFGPS erases the GPS IFD of TIFF data (the body of an EXIF block)
// in place, keeping every length and offset. GPS pointers are looked for in
// IFD0, its Exif sub-IFD and IFD1. If a GPS entry cannot be followed safely
// the whole block is zeroed instead, so nothing can survive. It reports
// whether anything changed.
func blankTIFFGPS(b []byte) bool {
	if len(b) < 8 {
		return false
	}
	t := tiff{b: b}
	switch string(b[:2]) {
	case "II":
		t.order = binary.LittleEndian
	case "MM":
		t.order = binary.BigEndian
	default:
		return false
	}
	ifd0, ok := t.u32(4)
	if !ok {
		return false
	}

	changed, bad := false, false
	// scan blanks the GPS IFDs referenced from the IFD at off and returns
	// the offset of the next IFD (0 when none or unreadable).
	var scan func(off, depth int) int
	scan = func(off, depth int) int {
		n, ok := t.u16(off)
		if !ok {
			return 0
		}
		for i := range n {
			e := off + 2 + i*12
			if e+12 > len(b) {
				bad = true
				return 0
			}
			tag, _ := t.u16(e)
			if tag != 0x8825 && tag != 0x8769 {
				continue
			}
			// A pointer is one LONG (or IFD) held in the entry. Any other
			// form makes readers look elsewhere, so it cannot be followed
			// safely.
			if typ, _ := t.u16(e + 2); typ != 4 && typ != 13 {
				bad = true
				continue
			}
			if count, _ := t.u32(e + 4); count != 1 {
				bad = true
				continue
			}
			p, _ := t.u32(e + 8)
			if tag == 0x8825 {
				c, ok := t.blankGPSIFD(p)
				changed = changed || c
				bad = bad || !ok
			} else if depth == 0 {
				scan(p, depth+1)
			}
		}
		next, _ := t.u32(off + 2 + n*12)
		return next
	}
	if ifd1 := scan(ifd0, 0); ifd1 > 0 {
		scan(ifd1, 1)
	}
	if bad {
		clear(b)
		return true
	}
	return changed
}

// blankGPSIFD zeroes the entries and out-of-line values of the IFD at off and
// sets its entry count to zero. ok is false when the IFD runs out of bounds.
func (t tiff) blankGPSIFD(off int) (changed, ok bool) {
	n, ok := t.u16(off)
	if !ok {
		return false, false
	}
	if n == 0 {
		return false, true
	}
	if off+2+n*12 > len(t.b) {
		return false, false
	}
	for i := range n {
		e := off + 2 + i*12
		typ, _ := t.u16(e + 2)
		count, _ := t.u32(e + 4)
		unit, known := allTypeSizes[typ]
		if !known {
			unit = 1
		}
		if size := unit * count; size > 4 {
			p, _ := t.u32(e + 8)
			if p < 0 || p+size > len(t.b) {
				return false, false
			}
			clear(t.b[p : p+size])
		}
	}
	clear(t.b[off+2 : off+2+n*12])
	t.order.PutUint16(t.b[off:], 0)
	return true, true
}

// allTypeSizes extends typeSize with the TIFF types ReadMeta does not use,
// so their out-of-line values are found and cleared too.
var allTypeSizes = map[int]int{1: 1, 2: 1, 3: 2, 4: 4, 5: 8, 6: 1, 7: 1, 8: 2, 9: 4, 10: 8, 11: 4, 12: 8, 13: 4, 16: 8, 17: 8, 18: 8}

var (
	xmpGPSAttr = regexp.MustCompile(`[\w.-]+:(?i:gps)\w*\s*=\s*("[^"]*"|'[^']*')`)
	xmpGPSOpen = regexp.MustCompile(`<([\w.-]+:(?i:gps)\w*)\b[^>]*>`)
)

// blankXMPLocation replaces the values of GPS properties in XMP (exif:GPS*,
// drone-dji:GpsLatitude, …) with spaces, in place, so the XML stays
// well-formed and the length unchanged. Other properties, such as face
// regions, are left alone. It reports whether anything changed.
func blankXMPLocation(b []byte) bool {
	changed := false
	blank := func(from, to int) {
		for i := from; i < to; i++ {
			if b[i] != ' ' {
				b[i] = ' '
				changed = true
			}
		}
	}
	for _, m := range xmpGPSAttr.FindAllSubmatchIndex(b, -1) {
		blank(m[2]+1, m[3]-1) // between the quotes
	}
	for _, m := range xmpGPSOpen.FindAllSubmatchIndex(b, -1) {
		if b[m[1]-2] == '/' {
			continue // <exif:GPSLatitude/>: no content
		}
		name := b[m[2]:m[3]]
		end := bytes.Index(b[m[1]:], append([]byte("</"), name...))
		if end < 0 {
			blank(m[1], len(b)) // unterminated: fail closed
			continue
		}
		blank(m[1], m[1]+end)
	}
	return changed
}

var (
	exifPrefix   = []byte("Exif\x00\x00")
	xmpPrefix    = []byte("http://ns.adobe.com/xap/1.0/\x00")
	xmpExtPrefix = []byte("http://ns.adobe.com/xmp/extension/\x00")
)

// blankExifBlock handles EXIF data that may carry the "Exif\0\0" prefix.
func blankExifBlock(b []byte) bool {
	return blankTIFFGPS(bytes.TrimPrefix(b, exifPrefix))
}

// stripJPEG blanks the GPS position in the EXIF and XMP segments of an open
// JPEG, in place. Segments are rewritten with the same length.
func stripJPEG(f *os.File) (bool, error) {
	changed, stop, _, err := walkJPEG(f, 2)
	if err != nil {
		return changed, err
	}
	c, err := stripEmbeddedJPEGs(f, stop)
	return changed || c, err
}

// maxEmbedded bounds how many embedded images are walked; a file with more
// is not trusted.
const maxEmbedded = 1000

// walkJPEG blanks the position in the EXIF and XMP segments of the image
// whose header segments start at start (just after its SOI). It stops at the
// start of image data (SOS) or EOI, and reports where in stop; clean is false
// when the walk ended any other way (garbage, a cut-off segment).
func walkJPEG(f *os.File, start int64) (changed bool, stop int64, clean bool, err error) {
	var hdr [4]byte
	for pos := start; ; {
		if _, err := f.ReadAt(hdr[:], pos); err != nil {
			return changed, pos, false, nil // end of file before image data
		}
		if hdr[0] != 0xFF {
			return changed, pos, false, nil
		}
		marker := hdr[1]
		if marker == 0xD8 || (marker >= 0xD0 && marker <= 0xD7) || marker == 0x01 || marker == 0xFF {
			pos++
			continue
		}
		if marker == 0xDA || marker == 0xD9 {
			return changed, pos, true, nil
		}
		size := int64(binary.BigEndian.Uint16(hdr[2:]))
		if size < 2 {
			return changed, pos, false, nil
		}
		if marker == 0xE1 {
			payload := make([]byte, size-2)
			if n, err := f.ReadAt(payload, pos+4); err != nil {
				// A truncated segment ends the file; ReadMeta ignores it, but
				// other tools may read what is there, so it is cleared.
				changed, err = blankTail(f, n, pos+4, changed)
				return changed, pos, false, err
			}
			var c bool
			switch {
			case bytes.HasPrefix(payload, exifPrefix):
				c = blankTIFFGPS(payload[len(exifPrefix):])
			case bytes.HasPrefix(payload, xmpPrefix):
				c = blankXMPLocation(payload[len(xmpPrefix):])
			case bytes.HasPrefix(payload, xmpExtPrefix):
				// Extended XMP continues a larger XMP packet in pieces that
				// cannot be parsed one by one: blank it all.
				c = blankAll(payload[len(xmpExtPrefix):], ' ')
			}
			if c {
				if _, err := f.WriteAt(payload, pos+4); err != nil {
					return changed, pos, false, err
				}
				changed = true
			}
		}
		pos += 2 + size
	}
}

// blankAll fills b with c and reports whether that changed it.
func blankAll(b []byte, c byte) bool {
	changed := false
	for i := range b {
		if b[i] != c {
			b[i], changed = c, true
		}
	}
	return changed
}

// stripEmbeddedJPEGs handles images stored after the main one (the second
// image of MPO files, gain maps, previews), which carry their own EXIF.
// JPEG entropy data never contains FF D8, so every FF D8 FF after from is a
// start of image. Each one's header segments are walked like the main
// image's; if one cannot be parsed, or there are too many, everything after
// the first EOI is zeroed.
func stripEmbeddedJPEGs(f *os.File, from int64) (bool, error) {
	size, err := fileSize(f)
	if err != nil {
		return false, err
	}
	changed := false
	cursor := from
	for count := 0; ; count++ {
		at, ok, err := indexAt(f, cursor, size, []byte{0xFF, 0xD8, 0xFF})
		if err != nil || !ok {
			return changed, err
		}
		c, _, clean, err := walkJPEG(f, at+2)
		changed = changed || c
		if err != nil {
			return changed, err
		}
		if !clean || count >= maxEmbedded {
			eoi, found, err := indexAt(f, from, size, []byte{0xFF, 0xD9})
			if err != nil {
				return changed, err
			}
			if !found {
				eoi = from
			}
			if err := zeroRange(f, eoi+2, size); err != nil {
				return changed, err
			}
			return true, nil
		}
		cursor = at + 2
	}
}

func fileSize(f *os.File) (int64, error) {
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}

// indexAt finds the first occurrence of pat in [from, size) of f.
func indexAt(f *os.File, from, size int64, pat []byte) (int64, bool, error) {
	const block = 1 << 20
	buf := make([]byte, block+len(pat)-1)
	for pos := from; pos < size; pos += block {
		n, err := f.ReadAt(buf, pos)
		if err != nil && !errors.Is(err, io.EOF) {
			return 0, false, err
		}
		if i := bytes.Index(buf[:n], pat); i >= 0 {
			return pos + int64(i), true, nil
		}
	}
	return 0, false, nil
}

// zeroRange overwrites [from, to) of f with zeros.
func zeroRange(f *os.File, from, to int64) error {
	zeros := make([]byte, 64<<10)
	for from < to {
		n := min(int64(len(zeros)), to-from)
		if _, err := f.WriteAt(zeros[:n], from); err != nil {
			return err
		}
		from += n
	}
	return nil
}

// blankTail zeroes the n bytes at off, the rest of a file that ends inside
// a metadata block.
func blankTail(f *os.File, n int, off int64, changed bool) (bool, error) {
	if n == 0 {
		return changed, nil
	}
	if _, err := f.WriteAt(make([]byte, n), off); err != nil {
		return changed, err
	}
	return true, nil
}

var pngSignature = []byte("\x89PNG\r\n\x1a\n")

// stripPNG blanks the GPS position in the eXIf chunk and the XMP text chunk
// of an open PNG, in place, and fixes the CRCs.
func stripPNG(f *os.File) (bool, error) {
	changed := false
	var hdr [8]byte
	for pos := int64(len(pngSignature)); ; {
		if _, err := f.ReadAt(hdr[:], pos); err != nil {
			return changed, nil
		}
		size := int64(binary.BigEndian.Uint32(hdr[:4]))
		typ := string(hdr[4:])
		if typ == "IEND" {
			return changed, nil
		}
		dataAt := pos + 8
		switch typ {
		case "tEXt", "zTXt", "iTXt", "eXIf":
			var head [96]byte
			n, _ := f.ReadAt(head[:min(size, int64(len(head)))], dataAt)
			if typ != "eXIf" && pngMetaKeyword(head[:n]) && !(typ == "iTXt" && bytes.HasPrefix(head[:n], []byte("XML:com.adobe.xmp\x00"))) {
				// Text chunks that carry EXIF or XMP as text: nothing to parse,
				// the chunk becomes a private one that decoders skip.
				if err := neutralizePNGChunk(f, pos, size); err != nil {
					return changed, err
				}
				changed = true
				break
			}
			if typ == "tEXt" || typ == "zTXt" || (typ == "iTXt" && !bytes.HasPrefix(head[:n], []byte("XML:com.adobe.xmp\x00"))) {
				break
			}
			if size > maxChunk {
				return changed, errMetaTooLarge
			}
			data := make([]byte, size)
			if n, err := f.ReadAt(data, dataAt); err != nil {
				return blankTail(f, n, dataAt, changed)
			}
			var c bool
			if typ == "eXIf" {
				c = blankExifBlock(data)
			} else {
				c = blankITXtXMP(data)
			}
			if c {
				crc := crc32.NewIEEE()
				crc.Write(hdr[4:])
				crc.Write(data)
				var sum [4]byte
				binary.BigEndian.PutUint32(sum[:], crc.Sum32())
				if _, err := f.WriteAt(data, dataAt); err != nil {
					return changed, err
				}
				if _, err := f.WriteAt(sum[:], dataAt+size); err != nil {
					return changed, err
				}
				changed = true
			}
		}
		pos = dataAt + size + 4
	}
}

// pngMetaKeywords name text chunks that hold a whole EXIF or XMP block as text.
var pngMetaKeywords = []string{"Raw profile type exif", "Raw profile type APP1", "Raw profile type xmp", "XML:com.adobe.xmp"}

// pngMetaKeyword reports whether a text chunk's data starts with one of them.
func pngMetaKeyword(data []byte) bool {
	kw, _, ok := bytes.Cut(data, []byte{0})
	if !ok {
		return false
	}
	for _, k := range pngMetaKeywords {
		if strings.EqualFold(string(kw), k) {
			return true
		}
	}
	return false
}

// neutralizePNGChunk zeroes the data of the chunk at pos, renames it to gtPv
// (ancillary, private, safe to copy: decoders ignore it) and fixes the CRC.
// The length does not change.
func neutralizePNGChunk(f *os.File, pos, size int64) error {
	dataAt := pos + 8
	if fsize, err := fileSize(f); err != nil {
		return err
	} else if dataAt+size+4 > fsize {
		// The file ends inside the chunk: clear what is there, never grow the file.
		return zeroRange(f, dataAt, fsize)
	}
	if err := zeroRange(f, dataAt, dataAt+size); err != nil {
		return err
	}
	typ := []byte("gtPv")
	crc := crc32.NewIEEE()
	crc.Write(typ)
	zeros := make([]byte, 64<<10)
	for left := size; left > 0; {
		n := min(left, int64(len(zeros)))
		crc.Write(zeros[:n])
		left -= n
	}
	var sum [4]byte
	binary.BigEndian.PutUint32(sum[:], crc.Sum32())
	if _, err := f.WriteAt(typ, pos+4); err != nil {
		return err
	}
	_, err := f.WriteAt(sum[:], dataAt+size)
	return err
}

// blankITXtXMP blanks location in an iTXt chunk holding XMP. The layout is
// keyword NUL, compression flag, method, language NUL, translated keyword
// NUL, text. A compressed text that has a location (or cannot be
// inspected) is replaced by blank uncompressed text of the same length.
func blankITXtXMP(data []byte) bool {
	i := bytes.IndexByte(data, 0)
	if i < 0 || i+3 > len(data) {
		return false
	}
	flag := i + 1
	rest := data[i+3:]
	lang := bytes.IndexByte(rest, 0)
	if lang < 0 {
		return false
	}
	trans := bytes.IndexByte(rest[lang+1:], 0)
	if trans < 0 {
		return false
	}
	text := data[i+3+lang+1+trans+1:]
	if data[flag] == 0 {
		return blankXMPLocation(text)
	}
	zr, err := zlib.NewReader(bytes.NewReader(text))
	if err == nil {
		var plain []byte
		plain, err = io.ReadAll(io.LimitReader(zr, 4<<20+1))
		if err == nil && len(plain) <= 4<<20 && !blankXMPLocation(plain) {
			return false
		}
	}
	data[flag], data[flag+1] = 0, 0
	for j := range text {
		text[j] = ' '
	}
	return true
}

// stripWebP blanks the GPS position in the EXIF and XMP chunks of an open
// WebP, in place.
func stripWebP(f *os.File) (bool, error) {
	var riff [12]byte
	if _, err := f.ReadAt(riff[:], 0); err != nil || string(riff[:4]) != "RIFF" || string(riff[8:]) != "WEBP" {
		return false, nil
	}
	changed := false
	var hdr [8]byte
	for pos := int64(12); ; {
		if _, err := f.ReadAt(hdr[:], pos); err != nil {
			return changed, nil
		}
		size := int64(binary.LittleEndian.Uint32(hdr[4:]))
		typ := string(hdr[:4])
		dataAt := pos + 8
		if typ == "EXIF" || typ == "XMP " {
			if size > maxChunk {
				return changed, errMetaTooLarge
			}
			data := make([]byte, size)
			if _, err := f.ReadAt(data, dataAt); err != nil {
				return changed, fmt.Errorf("read webp chunk: %w", err)
			}
			var c bool
			if typ == "EXIF" {
				c = blankExifBlock(data)
			} else {
				c = blankXMPLocation(data)
			}
			if c {
				if _, err := f.WriteAt(data, dataAt); err != nil {
					return changed, err
				}
				changed = true
			}
		}
		pos = dataAt + size + size%2
	}
}
