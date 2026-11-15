package legacyseed

import (
	"bytes"
	"cmp"
	"context"
	"encoding/binary"
	"image"
	"image/png"
	"slices"

	"github.com/librecosta/gofurry/services/api/internal/storage"
	ico "github.com/sergeymakinen/go-ico"
)

const (
	logoSVG = "svg"
	logoICO = "ico"
)

func legacyLogoFormat(raw []byte) string {
	if bytes.HasPrefix(raw, []byte{0, 0, 1, 0}) {
		return logoICO
	}
	if bytes.HasPrefix(bytes.TrimSpace(bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))), []byte("<")) {
		return logoSVG
	}
	return ""
}

func recoverLogo(ctx context.Context, raw []byte, row logo, dest packageWriter, renderer string) (logo, error) {
	format := legacyLogoFormat(raw)
	var img image.Image
	var err error
	switch format {
	case logoSVG:
		size, err := validateSVG(raw)
		if err != nil {
			row.Status = "logo_svg_rejected"
			return row, nil
		}
		img = renderSVG(ctx, renderer, size)
	case logoICO:
		img = decodeICO(raw)
	}
	if ctx.Err() != nil {
		return row, ctx.Err()
	}
	if img == nil {
		row.Status = "logo_decode_failed"
		return row, nil
	}
	if !usableLogoPixels(img) {
		row.Status = "logo_quality_rejected"
		return row, nil
	}
	var asset storage.SiteAsset
	raw, asset, err = encodeLogo(ctx, img)
	if err != nil {
		if ctx.Err() != nil {
			return row, ctx.Err()
		}
		row.Status = "conversion_failed"
		return row, nil
	}
	return writeLogo(row, raw, asset, format+"_webp_centered", dest)
}

// Directory sizes are at most 256px. Validate the embedded image headers
// before calling a decoder, and check each frame independently.
func decodeICO(raw []byte) image.Image {
	if len(raw) < 6 {
		return nil
	}
	count := int(binary.LittleEndian.Uint16(raw[4:6]))
	end := 6 + 16*count
	if count < 1 || count > 32 || end > len(raw) {
		return nil
	}
	type frame struct {
		img  image.Image
		bits int
	}
	var frames []frame
	for i := range count {
		entry := raw[6+16*i : 22+16*i]
		size := int64(binary.LittleEndian.Uint32(entry[8:12]))
		offset := int64(binary.LittleEndian.Uint32(entry[12:16]))
		if offset < int64(end) || size < 1 || offset+size > int64(len(raw)) {
			continue
		}
		width, height := int(entry[0]), int(entry[1])
		if width == 0 {
			width = 256
		}
		if height == 0 {
			height = 256
		}
		img, bits := decodeICOFrame(entry, raw[offset:offset+size], width, height)
		if img != nil {
			frames = append(frames, frame{img, bits})
		}
	}
	slices.SortStableFunc(frames, func(a, b frame) int {
		as, bs := a.img.Bounds().Size(), b.img.Bounds().Size()
		return cmp.Or(cmp.Compare(bs.X*bs.Y, as.X*as.Y), cmp.Compare(b.bits, a.bits))
	})
	for _, f := range frames {
		if usableLogoPixels(f.img) {
			return f.img
		}
	}
	if len(frames) > 0 {
		return frames[0].img
	}
	return nil
}

func decodeICOFrame(entry, frame []byte, width, height int) (img image.Image, bits int) {
	defer func() {
		if recover() != nil {
			img, bits = nil, 0
		}
	}()
	if bytes.HasPrefix(frame, []byte("\x89PNG")) {
		if !validImageEnvelope(frame) {
			return nil, 0
		}
		shape, err := png.DecodeConfig(bytes.NewReader(frame))
		if err != nil || shape.Width != width || shape.Height != height {
			return nil, 0
		}
		img, err = png.Decode(bytes.NewReader(frame))
		if err != nil {
			return nil, 0
		}
		return img, 32
	}
	if len(frame) < 40 || binary.LittleEndian.Uint32(frame) != 40 ||
		binary.LittleEndian.Uint32(frame[4:]) != uint32(width) ||
		binary.LittleEndian.Uint32(frame[8:]) != uint32(height*2) ||
		binary.LittleEndian.Uint16(frame[12:]) != 1 || binary.LittleEndian.Uint32(frame[16:]) != 0 {
		return nil, 0
	}
	bits = int(binary.LittleEndian.Uint16(frame[14:]))
	if bits != 1 && bits != 4 && bits != 8 && bits != 24 && bits != 32 {
		return nil, 0
	}
	palette := int64(binary.LittleEndian.Uint32(frame[32:]))
	if bits <= 8 && palette == 0 {
		palette = 1 << bits
	}
	if palette > 256 || bits <= 8 && palette > 1<<bits {
		return nil, 0
	}
	xorBytes := int64((width*bits + 31) / 32 * 4 * height)
	andBytes := int64((width + 31) / 32 * 4 * height)
	if int64(len(frame)) != 40+palette*4+xorBytes+andBytes {
		return nil, 0
	}
	header := []byte{0, 0, 1, 0, 1, 0}
	header = append(header, entry...)
	binary.LittleEndian.PutUint32(header[18:], uint32(len(header)))
	img, err := ico.Decode(bytes.NewReader(append(header, frame...)))
	if err != nil || img.Bounds().Size() != image.Pt(width, height) {
		return nil, 0
	}
	return img, bits
}

func usableLogoPixels(img image.Image) bool {
	bounds := img.Bounds()
	if min(bounds.Dx(), bounds.Dy()) < 8 {
		return false
	}
	visible, distinct := 0, false
	var first [4]uint32
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a < 16*257 {
				distinct = true
				continue
			}
			current := [4]uint32{r, g, b, a}
			if visible == 0 {
				first = current
			}
			distinct = distinct || first != current
			visible++
		}
	}
	return distinct && visible >= max(8, bounds.Dx()*bounds.Dy()/500)
}
