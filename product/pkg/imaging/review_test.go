package imaging

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// jpegWithOrientation encodes a w×h JPEG and inserts an EXIF APP1 segment
// carrying the orientation tag and a GPS IFD pointer, as a phone writes it.
func jpegWithOrientation(t *testing.T, w, h, orientation int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 0, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	src := buf.Bytes()

	tiff := make([]byte, 0, 64)
	tiff = append(tiff, 'M', 'M', 0, 42, 0, 0, 0, 8)
	tiff = binary.BigEndian.AppendUint16(tiff, 2) // two entries
	// Orientation (0x0112), SHORT, count 1, value
	tiff = binary.BigEndian.AppendUint16(tiff, 0x0112)
	tiff = binary.BigEndian.AppendUint16(tiff, 3)
	tiff = binary.BigEndian.AppendUint32(tiff, 1)
	tiff = binary.BigEndian.AppendUint16(tiff, uint16(orientation))
	tiff = append(tiff, 0, 0)
	// GPSInfo (0x8825), LONG, count 1, offset (points nowhere useful)
	tiff = binary.BigEndian.AppendUint16(tiff, 0x8825)
	tiff = binary.BigEndian.AppendUint16(tiff, 4)
	tiff = binary.BigEndian.AppendUint32(tiff, 1)
	tiff = binary.BigEndian.AppendUint32(tiff, 0)
	tiff = binary.BigEndian.AppendUint32(tiff, 0) // next IFD

	payload := append([]byte("Exif\x00\x00"), tiff...)
	app1 := []byte{0xFF, 0xE1}
	app1 = binary.BigEndian.AppendUint16(app1, uint16(len(payload)+2))
	app1 = append(app1, payload...)

	out := append([]byte{}, src[:2]...)
	out = append(out, app1...)
	return append(out, src[2:]...)
}

func TestReviewJPEGs_TurnsPhoneOrientationUpright(t *testing.T) {
	for _, c := range []struct {
		orientation  int
		wantW, wantH int
	}{
		{1, 40, 20}, {3, 40, 20}, {6, 20, 40}, {8, 20, 40},
	} {
		data := jpegWithOrientation(t, 40, 20, c.orientation)
		if got := exifOrientation(data); got != c.orientation {
			t.Fatalf("exifOrientation = %d, want %d", got, c.orientation)
		}
		full, thumb, err := ReviewJPEGs(data)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(full))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Width != c.wantW || cfg.Height != c.wantH {
			t.Fatalf("orientation %d: %dx%d, want %dx%d", c.orientation, cfg.Width, cfg.Height, c.wantW, c.wantH)
		}
		for _, out := range [][]byte{full, thumb} {
			if bytes.Contains(out, []byte("Exif")) {
				t.Fatalf("orientation %d: EXIF survived re-encoding", c.orientation)
			}
		}
	}
}

func TestReviewJPEGs_ScalesDownLongEdge(t *testing.T) {
	data := jpegWithOrientation(t, 2000, 1000, 6)
	full, thumb, err := ReviewJPEGs(data)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := jpeg.DecodeConfig(bytes.NewReader(full))
	th, _ := jpeg.DecodeConfig(bytes.NewReader(thumb))
	if f.Width != 800 || f.Height != ReviewMaxEdge || th.Height != ReviewThumbMaxEdge {
		t.Fatalf("full %dx%d thumb %dx%d", f.Width, f.Height, th.Width, th.Height)
	}
}

func TestReviewJPEGs_RefusesGarbage(t *testing.T) {
	if _, _, err := ReviewJPEGs([]byte("not an image")); err == nil {
		t.Fatal("garbage accepted")
	}
	if _, _, err := ReviewJPEGs(nil); err == nil {
		t.Fatal("empty accepted")
	}
}
