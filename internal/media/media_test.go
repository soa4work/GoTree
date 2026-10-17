package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestDetect(t *testing.T) {
	jpg := encodeJPEG(t, 4, 4)
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		head []byte
		file string
		mime string
		kind Kind
		ok   bool
	}{
		{"jpeg", jpg, "a.jpg", "image/jpeg", KindImage, true},
		{"png", pngBuf.Bytes(), "a.png", "image/png", KindImage, true},
		{"pdf", []byte("%PDF-1.7\n"), "a.pdf", "application/pdf", KindDocument, true},
		{"mp3 id3", []byte("ID3\x04\x00\x00\x00\x00\x00\x00"), "a.mp3", "audio/mpeg", KindAudio, true},
		{"m4a", append([]byte{0, 0, 0, 0x20}, []byte("ftypM4A \x00\x00\x00\x00")...), "a.m4a", "audio/mp4", KindAudio, true},
		{"mp4", append([]byte{0, 0, 0, 0x20}, []byte("ftypisom\x00\x00\x00\x00")...), "a.mp4", "video/mp4", KindVideo, true},
		{"wav", []byte("RIFF\x00\x00\x00\x00WAVEfmt "), "a.wav", "audio/wav", KindAudio, true},
		{"ogg", []byte("OggS\x00\x02\x00\x00\x00\x00\x00\x00"), "a.ogg", "audio/ogg", KindAudio, true},
		{"svg is refused", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), "a.svg", "", "", false},
		{"html is refused", []byte("<!DOCTYPE html><html>"), "a.jpg", "", "", false},
		{"text is refused", []byte("hello"), "a.txt", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mime, kind, ok := Detect(tt.head, tt.file)
			if ok != tt.ok || (ok && (mime != tt.mime || kind != tt.kind)) {
				t.Errorf("Detect = %q %q %v, want %q %q %v", mime, kind, ok, tt.mime, tt.kind, tt.ok)
			}
		})
	}
}

func TestSave(t *testing.T) {
	f := Files{Root: t.TempDir()}
	a, err := f.Save(strings.NewReader("hello"), 100)
	if err != nil {
		t.Fatal(err)
	}
	if a.Size != 5 || a.SHA256 != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Errorf("stored %+v", a)
	}
	b, err := f.Save(strings.NewReader("hello"), 100)
	if err != nil || b != a {
		t.Errorf("second save %+v, %v", b, err)
	}
	p, _ := f.Path(a.SHA256)
	if data, err := os.ReadFile(p); err != nil || string(data) != "hello" {
		t.Errorf("file content %q, %v", data, err)
	}
	if _, err := f.Save(strings.NewReader("too long"), 3); !errors.Is(err, ErrTooLarge) {
		t.Errorf("limit: got %v", err)
	}
	if _, err := f.Path("../../etc/passwd"); err == nil {
		t.Error("path traversal through the hash must be rejected")
	}
	if err := f.Remove(a.SHA256); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Error("file not removed")
	}
}

func TestReadMeta(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		t.Run(order.String(), func(t *testing.T) {
			exif := buildEXIF(order, 6, "1975:03:12 14:30:00", 'N', [3]float64{51, 20, 24}, 'E', [3]float64{12, 22, 30})
			m := ReadMeta(bytes.NewReader(withSegments(encodeJPEG(t, 8, 8), exif)))
			if m.Orientation != 6 || m.TakenAt != "1975-03-12 14:30:00" {
				t.Errorf("meta %+v", m)
			}
			if m.Lat == nil || math.Abs(*m.Lat-51.34) > 1e-6 || math.Abs(*m.Lng-12.375) > 1e-6 {
				t.Errorf("gps %v %v", m.Lat, m.Lng)
			}
		})
	}

	south := buildEXIF(binary.LittleEndian, 1, "", 'S', [3]float64{33, 52, 0}, 'W', [3]float64{70, 0, 0})
	m := ReadMeta(bytes.NewReader(withSegments(encodeJPEG(t, 8, 8), south)))
	if m.Lat == nil || *m.Lat >= 0 || *m.Lng >= 0 {
		t.Errorf("southern/western coordinates must be negative: %v %v", m.Lat, m.Lng)
	}

	plain := ReadMeta(bytes.NewReader(encodeJPEG(t, 8, 8)))
	if plain.Orientation != 1 || plain.TakenAt != "" || plain.Lat != nil || plain.Regions != nil {
		t.Errorf("no metadata: %+v", plain)
	}
}

const xmpAttributes = `<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
<rdf:Description xmlns:mwg-rs="http://www.metadataworkinggroup.com/schemas/regions/"
  xmlns:stArea="http://ns.adobe.com/xmp/sType/Area#" xmlns:stDim="http://ns.adobe.com/xap/1.0/sType/Dimensions#">
 <mwg-rs:Regions rdf:parseType="Resource">
  <mwg-rs:AppliedToDimensions stDim:w="4000" stDim:h="3000" stDim:unit="pixel"/>
  <mwg-rs:RegionList><rdf:Bag>
   <rdf:li><rdf:Description mwg-rs:Name="Anna Müller" mwg-rs:Type="Face">
     <mwg-rs:Area stArea:x="0.3" stArea:y="0.4" stArea:w="0.2" stArea:h="0.2" stArea:unit="normalized"/>
   </rdf:Description></rdf:li>
   <rdf:li><rdf:Description mwg-rs:Name="A pet" mwg-rs:Type="Pet">
     <mwg-rs:Area stArea:x="0.5" stArea:y="0.5" stArea:w="0.1" stArea:h="0.1" stArea:unit="normalized"/>
   </rdf:Description></rdf:li>
  </rdf:Bag></mwg-rs:RegionList>
 </mwg-rs:Regions>
</rdf:Description></rdf:RDF></x:xmpmeta>`

const xmpElements = `<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
<rdf:Description xmlns:mwg-rs="http://www.metadataworkinggroup.com/schemas/regions/" xmlns:stArea="http://ns.adobe.com/xmp/sType/Area#">
 <mwg-rs:Regions rdf:parseType="Resource"><mwg-rs:RegionList><rdf:Bag>
  <rdf:li rdf:parseType="Resource">
   <mwg-rs:Name>Hans Weber</mwg-rs:Name>
   <mwg-rs:Type>Face</mwg-rs:Type>
   <mwg-rs:Area rdf:parseType="Resource">
    <stArea:x>0.95</stArea:x><stArea:y>0.1</stArea:y><stArea:w>0.2</stArea:w><stArea:h>0.1</stArea:h>
    <stArea:unit>normalized</stArea:unit>
   </mwg-rs:Area>
  </rdf:li>
 </rdf:Bag></mwg-rs:RegionList></mwg-rs:Regions>
</rdf:Description></rdf:RDF></x:xmpmeta>`

func TestXMPRegions(t *testing.T) {
	m := ReadMeta(bytes.NewReader(withSegments(encodeJPEG(t, 8, 8), xmpSegment(xmpAttributes))))
	if len(m.Regions) != 1 {
		t.Fatalf("regions %+v (pets are not faces)", m.Regions)
	}
	r := m.Regions[0]
	if r.Name != "Anna Müller" || !near(r.X, 0.2) || !near(r.Y, 0.3) || !near(r.W, 0.2) || !near(r.H, 0.2) {
		t.Errorf("centered area must become top-left: %+v", r)
	}

	m = ReadMeta(bytes.NewReader(withSegments(encodeJPEG(t, 8, 8), xmpSegment(xmpElements))))
	if len(m.Regions) != 1 {
		t.Fatalf("regions %+v", m.Regions)
	}
	r = m.Regions[0]
	// The box sticks out on the right and is clipped to the image.
	if r.Name != "Hans Weber" || !near(r.X, 0.85) || !near(r.W, 0.15) || !near(r.Y, 0.05) {
		t.Errorf("element form: %+v", r)
	}
}

func TestReadMetaSurvivesGarbage(t *testing.T) {
	exif := buildEXIF(binary.BigEndian, 3, "2001:02:03 04:05:06", 'N', [3]float64{1, 2, 3}, 'E', [3]float64{4, 5, 6})
	full := withSegments(encodeJPEG(t, 8, 8), exif, xmpSegment(xmpAttributes))
	for cut := range len(full) {
		_ = ReadMeta(bytes.NewReader(full[:cut]))
	}
	// Corrupt bytes one at a time across the metadata.
	for i := 2; i < 400 && i < len(full); i++ {
		bad := bytes.Clone(full)
		bad[i] ^= 0xFF
		_ = ReadMeta(bytes.NewReader(bad))
	}
}

func TestOrient(t *testing.T) {
	// A 2x1 image: red on the left, blue on the right.
	src := image.NewRGBA(image.Rect(0, 0, 2, 1))
	red, blue := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}
	src.Set(0, 0, red)
	src.Set(1, 0, blue)
	at := func(img image.Image, x, y int) color.RGBA { return img.At(x, y).(color.RGBA) }

	if img := Orient(src, 3); at(img, 0, 0) != blue {
		t.Error("180°: blue should be on the left")
	}
	img := Orient(src, 6)
	if img.Bounds().Dx() != 1 || img.Bounds().Dy() != 2 || at(img, 0, 0) != red || at(img, 0, 1) != blue {
		t.Error("90° clockwise: red on top, blue below")
	}
	img = Orient(src, 8)
	if at(img, 0, 0) != blue || at(img, 0, 1) != red {
		t.Error("90° counter-clockwise: blue on top")
	}
	if Orient(src, 1) != image.Image(src) {
		t.Error("orientation 1 must return the image unchanged")
	}
}

func TestThumb(t *testing.T) {
	f := Files{Root: t.TempDir()}
	exif := buildEXIF(binary.LittleEndian, 6, "", 0, [3]float64{}, 0, [3]float64{})
	stored, err := f.Save(bytes.NewReader(withSegments(encodeJPEG(t, 400, 200), exif)), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	path, _ := f.Path(stored.SHA256)
	if w, h, err := DisplaySize(path, 6); err != nil || w != 200 || h != 400 {
		t.Errorf("display size %dx%d, %v", w, h, err)
	}

	thumbSize := func(p string) (int, int) {
		t.Helper()
		in, err := os.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		defer in.Close()
		cfg, err := jpeg.DecodeConfig(in)
		if err != nil {
			t.Fatal(err)
		}
		return cfg.Width, cfg.Height
	}
	p, err := f.Thumb(stored.SHA256, 128, 6, nil)
	if err != nil {
		t.Fatal(err)
	}
	if w, h := thumbSize(p); w != 64 || h != 128 {
		t.Errorf("rotated thumbnail is %dx%d, want 64x128", w, h)
	}
	again, err := f.Thumb(stored.SHA256, 128, 6, nil)
	if err != nil || again != p {
		t.Errorf("thumbnail not cached: %q vs %q, %v", again, p, err)
	}
	p, err = f.Thumb(stored.SHA256, 128, 6, &Crop{ID: 1, X: 0, Y: 0, W: 1, H: 0.25})
	if err != nil {
		t.Fatal(err)
	}
	if w, h := thumbSize(p); w != 128 || h != 64 {
		t.Errorf("cropped thumbnail is %dx%d, want 128x64", w, h)
	}
	if _, err := f.Thumb(stored.SHA256, 99, 1, nil); err == nil {
		t.Error("odd sizes must be refused")
	}
}

// hugePNG returns a valid PNG header and no pixel data, declaring w x h.
func hugePNG(w, h uint32) []byte {
	chunk := func(typ string, data []byte) []byte {
		var b bytes.Buffer
		_ = binary.Write(&b, binary.BigEndian, uint32(len(data)))
		b.WriteString(typ)
		b.Write(data)
		_ = binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(typ), data...)))
		return b.Bytes()
	}
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr, w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 6 // 8 bit RGBA
	return append(append([]byte("\x89PNG\r\n\x1a\n"), chunk("IHDR", ihdr)...), chunk("IDAT", nil)...)
}

func TestThumbRefusesHugeImage(t *testing.T) {
	f := Files{Root: t.TempDir()}
	stored, err := f.Save(bytes.NewReader(hugePNG(40000, 40000)), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	path, _ := f.Path(stored.SHA256)
	if err := CheckImageSize(path); !errors.Is(err, ErrImageTooLarge) {
		t.Errorf("check: %v, want ErrImageTooLarge", err)
	}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err = f.Thumb(stored.SHA256, 128, 6, nil)
	runtime.ReadMemStats(&after)
	if !errors.Is(err, ErrImageTooLarge) {
		t.Errorf("thumb: %v, want ErrImageTooLarge", err)
	}
	if got := after.TotalAlloc - before.TotalAlloc; got > 50<<20 {
		t.Errorf("refusing the image allocated %d MB", got>>20)
	}

	// Exactly at the limit still passes the check.
	ok, err := f.Save(bytes.NewReader(hugePNG(5000, 10000)), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	okPath, _ := f.Path(ok.SHA256)
	if err := CheckImageSize(okPath); err != nil {
		t.Errorf("50 megapixels: %v", err)
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func encodeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := range w {
		img.Set(x, 0, color.RGBA{uint8(x), 100, 200, 255})
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// withSegments inserts APP segments right after the SOI marker.
func withSegments(jpg []byte, segs ...[]byte) []byte {
	out := append([]byte{}, jpg[:2]...)
	for _, s := range segs {
		out = append(out, s...)
	}
	return append(out, jpg[2:]...)
}

func app1(payload []byte) []byte {
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	return append(seg, payload...)
}

func xmpSegment(xmp string) []byte {
	return app1(append([]byte("http://ns.adobe.com/xap/1.0/\x00"), xmp...))
}

// buildEXIF writes a minimal EXIF block: IFD0 with Orientation and
// pointers to an Exif IFD (DateTimeOriginal) and a GPS IFD. latRef 0
// omits GPS, an empty date omits the Exif IFD.
func buildEXIF(order binary.ByteOrder, orientation int, taken string, latRef byte, lat [3]float64, lngRef byte, lng [3]float64) []byte {
	type entry struct {
		tag, typ, count int
		value           []byte // inline when <= 4 bytes, else stored after the IFD
	}
	u16 := func(v int) []byte { b := make([]byte, 2); order.PutUint16(b, uint16(v)); return b }
	u32 := func(v int) []byte { b := make([]byte, 4); order.PutUint32(b, uint32(v)); return b }
	rationals := func(v [3]float64) []byte {
		var b []byte
		for _, f := range v {
			b = append(b, u32(int(f*1000))...)
			b = append(b, u32(1000)...)
		}
		return b
	}

	var buf []byte
	writeIFD := func(at int, entries []entry) []byte {
		ifd := u16(len(entries))
		dataOff := at + 2 + len(entries)*12 + 4
		var data []byte
		for _, e := range entries {
			ifd = append(ifd, u16(e.tag)...)
			ifd = append(ifd, u16(e.typ)...)
			ifd = append(ifd, u32(e.count)...)
			if len(e.value) <= 4 {
				v := append(append([]byte{}, e.value...), make([]byte, 4-len(e.value))...)
				ifd = append(ifd, v...)
			} else {
				ifd = append(ifd, u32(dataOff+len(data))...)
				data = append(data, e.value...)
			}
		}
		ifd = append(ifd, u32(0)...)
		return append(ifd, data...)
	}

	header := []byte("II")
	if order == binary.BigEndian {
		header = []byte("MM")
	}
	header = append(header, u16(42)...)
	header = append(header, u32(8)...)

	// Lay out: IFD0 at 8, then the Exif IFD, then the GPS IFD.
	ifd0Entries := []entry{{0x0112, 3, 1, u16(orientation)}}
	if taken != "" {
		ifd0Entries = append(ifd0Entries, entry{0x8769, 4, 1, nil})
	}
	if latRef != 0 {
		ifd0Entries = append(ifd0Entries, entry{0x8825, 4, 1, nil})
	}
	ifd0Size := len(writeIFD(8, ifd0Entries))
	exifAt := 8 + ifd0Size
	exifEntries := []entry{{0x9003, 2, len(taken) + 1, append([]byte(taken), 0)}}
	exifSize := 0
	if taken != "" {
		exifSize = len(writeIFD(exifAt, exifEntries))
	}
	gpsAt := exifAt + exifSize
	for i := range ifd0Entries {
		switch ifd0Entries[i].tag {
		case 0x8769:
			ifd0Entries[i].value = u32(exifAt)
		case 0x8825:
			ifd0Entries[i].value = u32(gpsAt)
		}
	}
	buf = append(header, writeIFD(8, ifd0Entries)...)
	if taken != "" {
		buf = append(buf, writeIFD(exifAt, exifEntries)...)
	}
	if latRef != 0 {
		buf = append(buf, writeIFD(gpsAt, []entry{
			{1, 2, 2, []byte{latRef, 0}},
			{2, 5, 3, rationals(lat)},
			{3, 2, 2, []byte{lngRef, 0}},
			{4, 5, 3, rationals(lng)},
		})...)
	}
	return app1(append([]byte("Exif\x00\x00"), buf...))
}
