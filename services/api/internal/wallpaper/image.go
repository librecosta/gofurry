package wallpaper

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"os"

	"github.com/gen2brain/gav1d/avif"
	"golang.org/x/image/webp"
)

const MaxImageBytes int64 = 50 << 20
const MaxPreviewBytes int64 = 5 << 20

type Asset struct {
	Key         string `json:"key"`
	URL         string `json:"url"`
	SHA256      string `json:"sha256"`
	Bytes       int64  `json:"bytes"`
	ContentType string `json:"content_type"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
}

// ValidateAsset consumes a private seekable file. No client-provided dimensions
// or filename enter the result. Callers bound concurrent complete decodes.
func ValidateAsset(ctx context.Context, f *os.File, contentType string, preview bool) (a Asset, err error) {
	defer func() {
		if recover() != nil {
			a = Asset{}
			err = ErrInvalid
		}
	}()
	st, e := f.Stat()
	limit := MaxImageBytes
	if preview {
		limit = MaxPreviewBytes
	}
	if e != nil || st.Size() < 12 || st.Size() > limit || ctx.Err() != nil {
		return a, ErrInvalid
	}
	if contentType != "image/avif" && contentType != "image/webp" || preview && contentType != "image/webp" {
		return a, ErrInvalid
	}
	size := st.Size()
	var cfg image.Config
	f.Seek(0, io.SeekStart)
	if contentType == "image/webp" {
		if checkWebP(f, size) != nil {
			return a, ErrInvalid
		}
		f.Seek(0, io.SeekStart)
		cfg, e = webp.DecodeConfig(f)
	} else {
		if checkAVIF(f, size) != nil {
			return a, ErrInvalid
		}
		f.Seek(0, io.SeekStart)
		cfg, e = avif.DecodeConfig(f)
	}
	pixels := int64(cfg.Width) * int64(cfg.Height)
	maxPixels := int64(36000000)
	maxSide := 16384
	if preview {
		maxPixels = 4000000
		maxSide = 4096
	}
	if e != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxSide || cfg.Height > maxSide || pixels > maxPixels || pixels > size*20000 {
		return a, ErrInvalid
	}
	f.Seek(0, io.SeekStart)
	var decoded image.Image
	if contentType == "image/webp" {
		decoded, e = webp.Decode(f)
	} else {
		decoded, e = avif.Decode(f, avif.Options{FrameSizeLimit: int(maxPixels), MaxFrames: 1})
	}
	if e != nil || decoded == nil || decoded.Bounds().Dx() != cfg.Width || decoded.Bounds().Dy() != cfg.Height || ctx.Err() != nil {
		return a, ErrInvalid
	}
	f.Seek(0, io.SeekStart)
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(f, limit+1))
	if e != nil || n != size {
		return a, ErrInvalid
	}
	f.Seek(0, io.SeekStart)
	hash := fmt.Sprintf("%x", h.Sum(nil))
	return Asset{Key: "start/backgrounds/" + hash + "." + contentType[6:], SHA256: hash, Bytes: size, ContentType: contentType, Width: cfg.Width, Height: cfg.Height}, nil
}
func checkWebP(r io.ReaderAt, size int64) error {
	var head [12]byte
	if _, e := r.ReadAt(head[:], 0); e != nil || string(head[:4]) != "RIFF" || string(head[8:]) != "WEBP" || int64(binary.LittleEndian.Uint32(head[4:8]))+8 != size {
		return ErrInvalid
	}
	coded := 0
	seen := map[string]bool{}
	for off := int64(12); off < size; {
		var hdr [8]byte
		if _, e := r.ReadAt(hdr[:], off); e != nil {
			return ErrInvalid
		}
		typ := string(hdr[:4])
		n := int64(binary.LittleEndian.Uint32(hdr[4:]))
		end := off + 8 + n + (n & 1)
		if end > size || n == 0 || seen[typ] {
			return ErrInvalid
		}
		seen[typ] = true
		switch typ {
		case "VP8 ", "VP8L":
			coded++
		case "VP8X":
			var flag [1]byte
			if n != 10 {
				return ErrInvalid
			}
			if _, e := r.ReadAt(flag[:], off+8); e != nil || flag[0]&0xce != 0 {
				return ErrInvalid
			}
		case "ALPH", "ICCP":
		default:
			return ErrInvalid // EXIF, XMP, animation and unknown appendages are not public-ready.
		}
		off = end
	}
	if coded != 1 {
		return ErrInvalid
	}
	return nil
}

// The maintained decoder validates the AV1 stream, item references, sizes and
// grids. This additional bounded ISO-BMFF walk excludes metadata and animation
// anywhere in the item directory, including unreferenced Exif/XMP items.
func checkAVIF(r io.ReaderAt, size int64) error {
	ftyp, meta, mdat := false, false, false
	err := boxes(r, 0, size, func(typ string, start, end int64) error {
		switch typ {
		case "ftyp":
			if ftyp || end-start < 8 || end-start > 128 || (end-start)%4 != 0 {
				return ErrInvalid
			}
			ftyp = true
			b := make([]byte, end-start)
			if _, e := r.ReadAt(b, start); e != nil {
				return e
			}
			valid := false
			for i := 0; i < len(b); i += 4 {
				if i == 4 {
					continue
				}
				brand := string(b[i : i+4])
				if brand == "avis" {
					return ErrInvalid
				}
				if brand == "avif" {
					valid = true
				}
			}
			if !valid {
				return ErrInvalid
			}
		case "meta":
			if meta || end-start > 1<<20 {
				return ErrInvalid
			}
			meta = true
			return boxes(r, start+4, end, func(t string, s, e int64) error {
				switch t {
				case "hdlr", "pitm", "iloc", "iprp", "iref", "idat", "grpl":
					return nil
				case "iinf":
					var h [8]byte
					if _, err := r.ReadAt(h[:], s); err != nil {
						return ErrInvalid
					}
					skip := int64(6)
					if h[0] == 1 {
						skip = 8
					} else if h[0] != 0 {
						return ErrInvalid
					}
					return boxes(r, s+skip, e, func(t string, s, e int64) error {
						if t != "infe" || e-s < 12 {
							return ErrInvalid
						}
						var b [14]byte
						if _, err := r.ReadAt(b[:min(int64(len(b)), e-s)], s); err != nil {
							return ErrInvalid
						}
						offset := 8
						if b[0] == 3 {
							offset = 10
						} else if b[0] != 2 {
							return ErrInvalid
						}
						if e-s < int64(offset+4) {
							return ErrInvalid
						}
						switch string(b[offset : offset+4]) {
						case "av01", "grid":
							return nil
						default:
							return ErrInvalid
						}
					})
				default:
					return ErrInvalid
				}
			})
		case "mdat":
			if mdat {
				return ErrInvalid
			}
			mdat = true
		case "free":
			if end-start > 1024 {
				return ErrInvalid
			}
			b := make([]byte, end-start)
			r.ReadAt(b, start)
			if !bytes.Equal(b, make([]byte, len(b))) {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
		return nil
	})
	if err != nil || !ftyp || !meta || !mdat {
		return ErrInvalid
	}
	return nil
}
func boxes(r io.ReaderAt, start, end int64, visit func(string, int64, int64) error) error {
	count := 0
	for off := start; off < end; {
		count++
		if count > 4096 || end-off < 8 {
			return ErrInvalid
		}
		var h [16]byte
		if _, e := r.ReadAt(h[:8], off); e != nil {
			return ErrInvalid
		}
		n := int64(binary.BigEndian.Uint32(h[:4]))
		header := int64(8)
		if n == 1 {
			if end-off < 16 {
				return ErrInvalid
			}
			if _, e := r.ReadAt(h[8:], off+8); e != nil {
				return ErrInvalid
			}
			u := binary.BigEndian.Uint64(h[8:])
			if u > uint64(end-off) {
				return ErrInvalid
			}
			n = int64(u)
			header = 16
		} else if n == 0 {
			n = end - off
		}
		if n < header || n > end-off {
			return ErrInvalid
		}
		if e := visit(string(h[4:8]), off+header, off+n); e != nil {
			return e
		}
		off += n
	}
	return nil
}
