package imaging

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/jpeg"
)

// Review photo sizes: the long edge of the full photo and of its thumbnail.
const (
	ReviewMaxEdge      = 1600
	ReviewThumbMaxEdge = 400
	ReviewJPEGQuality  = 85
	// ReviewMaxPixels refuses a canvas larger than 60 megapixels.
	ReviewMaxPixels = 60_000_000
)

// ReviewJPEGs decodes a shopper's photo, turns it upright, and re-encodes it
// as a full-size and a thumbnail JPEG.
//
// Re-encoding is also the privacy step: nothing of the original file survives,
// so its EXIF block — GPS position, device, time — never reaches the bucket.
// The orientation tag is read first and applied to the pixels, because a phone
// photo is stored sideways and relies on that tag to be shown upright.
func ReviewJPEGs(data []byte) (full, thumb []byte, err error) {
	if len(data) == 0 {
		return nil, nil, fmt.Errorf("empty image")
	}
	// Check the size before decoding: a small file can declare a huge canvas,
	// and decoding allocates all of it.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("decode image: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > ReviewMaxPixels {
		return nil, nil, fmt.Errorf("image is %dx%d, larger than allowed", cfg.Width, cfg.Height)
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("decode image: %w", err)
	}
	// Scale first, then turn upright: the long edge is the same either way,
	// and rotating the smaller image is much cheaper.
	upright := applyOrientation(fitMax(src, ReviewMaxEdge), exifOrientation(data))

	encode := func(img image.Image) ([]byte, error) {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: ReviewJPEGQuality}); err != nil {
			return nil, fmt.Errorf("encode jpeg: %w", err)
		}
		return buf.Bytes(), nil
	}
	if full, err = encode(upright); err != nil {
		return nil, nil, err
	}
	if thumb, err = encode(fitMax(upright, ReviewThumbMaxEdge)); err != nil {
		return nil, nil, err
	}
	return full, thumb, nil
}

// exifOrientation returns the EXIF orientation (1–8) of a JPEG, or 1 when the
// file is not a JPEG or carries none.
func exifOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	pos := 2
	for pos+4 <= len(data) {
		if data[pos] != 0xFF {
			return 1
		}
		marker := data[pos+1]
		if marker == 0xDA || marker == 0xD9 { // start of scan / end of image
			return 1
		}
		size := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
		if size < 2 || pos+2+size > len(data) {
			return 1
		}
		segment := data[pos+4 : pos+2+size]
		if marker == 0xE1 && len(segment) > 6 && string(segment[:6]) == "Exif\x00\x00" {
			return tiffOrientation(segment[6:])
		}
		pos += 2 + size
	}
	return 1
}

func tiffOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	ifd := int(order.Uint32(tiff[4:8]))
	if ifd < 8 || ifd+2 > len(tiff) {
		return 1
	}
	entries := int(order.Uint16(tiff[ifd : ifd+2]))
	for i := 0; i < entries; i++ {
		e := ifd + 2 + i*12
		if e+12 > len(tiff) {
			return 1
		}
		if order.Uint16(tiff[e:e+2]) == 0x0112 { // Orientation, SHORT
			v := int(order.Uint16(tiff[e+8 : e+10]))
			if v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

// applyOrientation returns src as it should be displayed for an EXIF
// orientation value.
func applyOrientation(src image.Image, orientation int) image.Image {
	if orientation <= 1 || orientation > 8 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	swap := orientation >= 5
	dw, dh := w, h
	if swap {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch orientation {
			case 2: // mirrored
				dx, dy = w-1-x, y
			case 3: // rotated 180
				dx, dy = w-1-x, h-1-y
			case 4: // mirrored vertically
				dx, dy = x, h-1-y
			case 5: // transposed
				dx, dy = y, x
			case 6: // rotated 90 clockwise to display
				dx, dy = h-1-y, x
			case 7: // transversed
				dx, dy = h-1-y, w-1-x
			case 8: // rotated 90 counter-clockwise to display
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}
