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
			switch tag {
			case 0x8825:
				p, _ := t.u32(e + 8)
				c, ok := t.blankGPSIFD(p)
				changed = changed || c
				bad = bad || !ok
			case 0x8769:
				if p, _ := t.u32(e + 8); depth == 0 {
					scan(p, depth+1)
				}
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
		unit, known := typeSize[typ]
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
	changed := false
	var hdr [4]byte
	for pos := int64(2); ; {
		if _, err := f.ReadAt(hdr[:], pos); err != nil {
			return changed, nil // end of file before image data: nothing more
		}
		if hdr[0] != 0xFF {
			return changed, nil
		}
		marker := hdr[1]
		if marker == 0xD8 || (marker >= 0xD0 && marker <= 0xD7) || marker == 0x01 || marker == 0xFF {
			pos++
			continue
		}
		if marker == 0xDA || marker == 0xD9 {
			return changed, nil
		}
		size := int64(binary.BigEndian.Uint16(hdr[2:]))
		if size < 2 {
			return changed, nil
		}
		if marker == 0xE1 {
			payload := make([]byte, size-2)
			if _, err := f.ReadAt(payload, pos+4); err != nil {
				return changed, nil // truncated segment; ReadMeta ignores it too
			}
			var c bool
			switch {
			case bytes.HasPrefix(payload, exifPrefix):
				c = blankTIFFGPS(payload[len(exifPrefix):])
			case bytes.HasPrefix(payload, xmpPrefix):
				c = blankXMPLocation(payload[len(xmpPrefix):])
			case bytes.HasPrefix(payload, xmpExtPrefix):
				c = blankXMPLocation(payload[len(xmpExtPrefix):])
			}
			if c {
				if _, err := f.WriteAt(payload, pos+4); err != nil {
					return changed, err
				}
				changed = true
			}
		}
		pos += 2 + size
	}
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
		case "eXIf", "iTXt":
			var head [32]byte
			n, _ := f.ReadAt(head[:min(size, int64(len(head)))], dataAt)
			if typ == "iTXt" && !bytes.HasPrefix(head[:n], []byte("XML:com.adobe.xmp\x00")) {
				break
			}
			if size > maxChunk {
				return changed, errMetaTooLarge
			}
			data := make([]byte, size)
			if _, err := f.ReadAt(data, dataAt); err != nil {
				return changed, nil
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
