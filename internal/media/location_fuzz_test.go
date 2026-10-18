package media

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

func fuzzSeedJPEG(f *testing.F) []byte {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewGray(image.Rect(0, 0, 8, 8)), nil); err != nil {
		f.Fatal(err)
	}
	return buf.Bytes()
}

// FuzzBlankTIFFGPS checks that blanking never panics, never changes the
// length, and leaves nothing ReadMeta would read as a position.
func FuzzBlankTIFFGPS(f *testing.F) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		exif := buildEXIF(order, 6, "2020:05:06 07:08:09", 'N', [3]float64{51, 20, 24}, 'E', [3]float64{12, 22, 12})
		f.Add(exif[4+6:])
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		n := len(b)
		blankTIFFGPS(b)
		if len(b) != n {
			t.Fatalf("length changed: %d -> %d", n, len(b))
		}
		var m Meta
		parseTIFF(b, &m)
		if m.Lat != nil || m.Lng != nil {
			t.Fatalf("position survived: %v %v", *m.Lat, *m.Lng)
		}
	})
}

// FuzzBlankXMPLocation checks that blanking keeps the length and is final:
// a second pass finds nothing left.
func FuzzBlankXMPLocation(f *testing.F) {
	f.Add([]byte(`<rdf:Description exif:GPSLatitude="51,20.4N"><exif:GPSLongitude>12,22.2E</exif:GPSLongitude><drone-dji:GpsAltitude/></rdf:Description>`))
	f.Add([]byte(`<x exif:gpsLat='1'><exif:GPSLongitude>unterminated`))
	f.Fuzz(func(t *testing.T, b []byte) {
		n := len(b)
		blankXMPLocation(b)
		if len(b) != n {
			t.Fatalf("length changed")
		}
		if blankXMPLocation(b) {
			t.Fatalf("second pass changed %q", b)
		}
	})
}

// FuzzStrip runs the file walkers on arbitrary files: no panic, same size,
// and a JPEG keeps no position ReadMeta can see.
func FuzzStrip(f *testing.F) {
	exif := buildEXIF(binary.LittleEndian, 6, "", 'N', [3]float64{51, 20, 24}, 'E', [3]float64{12, 22, 12})
	f.Add(withSegments(fuzzSeedJPEG(f), exif, xmpSegment(`<a exif:GPSLatitude="1"/>`)))
	f.Add(append([]byte("\x89PNG\r\n\x1a\n"), pngChunk("eXIf", exif[4+6:])...))
	f.Add(append([]byte("RIFF\x00\x00\x00\x00WEBPEXIF"), append(binary.LittleEndian.AppendUint32(nil, uint32(len(exif)-4)), exif[4:]...)...))
	dir := f.TempDir()
	f.Fuzz(func(t *testing.T, data []byte) {
		for i, strip := range []func(*os.File) (bool, error){stripJPEG, stripPNG, stripWebP} {
			p := filepath.Join(dir, "f")
			if err := os.WriteFile(p, data, 0o600); err != nil {
				t.Fatal(err)
			}
			fh, err := os.OpenFile(p, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			_, err = strip(fh)
			fh.Close()
			got, _ := os.ReadFile(p)
			if len(got) != len(data) {
				t.Fatalf("walker %d changed the size", i)
			}
			if i == 0 && err == nil {
				if m := ReadMeta(bytes.NewReader(got)); m.Lat != nil {
					t.Fatalf("position survived in JPEG")
				}
			}
		}
	})
}
