package legacyseed

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/HugoSmits86/nativewebp"
	"github.com/gen2brain/gav1d/avif"
	"github.com/librecosta/gofurry/services/api/internal/storage"
	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

const (
	imageInputLimit  = 8 << 20
	imagePixelsLimit = 16 << 20
	imageOutputLimit = 1 << 20
	logoReadLimit    = 5000
)

var logoKeyPattern = regexp.MustCompile(`^nav/sites/[1-9][0-9]{0,18}/icon/[a-f0-9]{32}(?:\.[a-z0-9]{1,16})?$`)
var outputLogoPattern = regexp.MustCompile(`^assets/site-logos/[a-f0-9]{64}\.webp$`)

type logo struct {
	SiteID     int64  `json:"legacy_site_id"`
	SourceKey  string `json:"source_object_key"`
	SHA        string `json:"output_sha256"`
	Path       string `json:"output_path"`
	Key        string `json:"output_key"`
	Status     string `json:"status"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Bytes      int64  `json:"byte_count"`
	Conversion string `json:"conversion"`
}

type logoReader struct {
	root    *os.Root
	inspect bool
	limit   int
	cache   map[string]logo
	seen    map[string]bool
}

func openPrivateFile(root *os.Root, path string) (*os.File, error) {
	if !filepath.IsLocal(path) {
		return nil, errors.New("invalid private file path")
	}
	partPath := ""
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == "" || part == "." || part == ".." {
			return nil, errors.New("invalid private file path")
		}
		partPath = filepath.Join(partPath, part)
		info, err := root.Lstat(partPath)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("linked private file path")
		}
	}
	info, err := root.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("invalid private file")
	}
	f, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() || !os.SameFile(info, fi) {
		f.Close()
		return nil, errors.New("changed private file")
	}
	return f, nil
}

func (l *logoReader) load(ctx context.Context, r *record, dest packageWriter) (logo, error) {
	key := r.Raw.str("icon")
	delete(r.Raw, "icon")
	row := logo{SiteID: r.Source.LegacyID, SourceKey: key}
	switch {
	case key == "":
		row.Status = "missing_icon"
	case !logoKeyPattern.MatchString(key):
		row.Status = "unsafe_key"
	case !logoBelongsTo(key, row.SiteID):
		row.Status = "logo_key_site_mismatch"
	case l.inspect:
		row.Status = "asset_not_inspected"
	case l.root == nil:
		row.Status = "asset_unavailable"
	default:
		var err error
		row, err = l.convert(ctx, key, dest)
		if err != nil {
			return logo{}, err
		}
		row.SiteID = r.Source.LegacyID
	}
	if row.Status == "ready" {
		if l.seen[row.SHA] {
			r.issue("logo_duplicate_content")
		}
		l.seen[row.SHA] = true
	} else if row.Status != "asset_not_inspected" {
		r.issue(row.Status)
	}
	return row, nil
}

func logoBelongsTo(key string, siteID int64) bool {
	return strings.HasPrefix(key, "nav/sites/"+strconv.FormatInt(siteID, 10)+"/icon/")
}

func (l *logoReader) convert(ctx context.Context, key string, dest packageWriter) (logo, error) {
	if row, ok := l.cache[key]; ok {
		return row, nil
	}
	row := logo{SourceKey: key, Status: "asset_limit_exceeded"}
	if len(l.cache) >= l.limit {
		return row, nil
	}
	row, err := convertObject(ctx, l.root, key, dest)
	if err == nil {
		l.cache[key] = row
	}
	return row, err
}

func convertObject(ctx context.Context, root *os.Root, key string, dest packageWriter) (logo, error) {
	row := logo{SourceKey: key, Status: "invalid_image"}
	f, err := openPrivateFile(root, key)
	if err != nil {
		row.Status = "object_read_rejected"
		if errors.Is(err, os.ErrNotExist) {
			row.Status = "missing_object"
		}
		return row, nil
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, imageInputLimit+1))
	if err != nil {
		row.Status = "object_read_rejected"
		return row, nil
	}
	if len(raw) > imageInputLimit {
		row.Status = "image_input_limit_exceeded"
		return row, nil
	}
	if ctx.Err() != nil {
		return row, ctx.Err()
	}
	// The old object writer used the first 32 hex characters of the original
	// SHA-256 in the filename. A stale/corrupted mirror file must fail even if
	// the replacement bytes happen to decode.
	if strings.SplitN(filepath.Base(key), ".", 2)[0] != sha(raw)[:32] {
		row.Status = "source_checksum_mismatch"
		return row, nil
	}
	if !validImageEnvelope(raw) {
		if !knownRaster(raw) {
			row.Status = "unsupported_image_format"
		}
		return row, nil
	}
	var validate *storage.Store
	asset, err := validate.ImportSiteAsset(ctx, storage.SiteLogo, bytes.NewReader(raw), true)
	conversion := "original_webp"
	if err != nil {
		img, err := decodeLogo(raw)
		if err != nil {
			return row, nil
		}
		row.Status = "conversion_failed"
		raw, asset, err = encodeLogo(ctx, img)
		if err != nil {
			return row, nil
		}
		conversion = "lossless_webp_centered"
	}
	row.Status, row.Conversion = "ready", conversion
	row.SHA, row.Key, row.Width, row.Height, row.Bytes = asset.SHA256, asset.Key, asset.Width, asset.Height, asset.Bytes
	row.Path = "assets/site-logos/" + row.SHA + ".webp"
	return row, dest.blob(row.Path, raw)
}

func validImageEnvelope(body []byte) bool {
	if len(body) < 12 {
		return false
	}
	switch {
	case string(body[:4]) == "RIFF":
		if string(body[8:12]) != "WEBP" || int64(binary.LittleEndian.Uint32(body[4:8]))+8 != int64(len(body)) {
			return false
		}
		data := body[12:]
		pictures := 0
		for len(data) > 0 {
			if len(data) < 8 {
				return false
			}
			n := int64(binary.LittleEndian.Uint32(data[4:8]))
			padded := n + n%2
			if padded+8 > int64(len(data)) {
				return false
			}
			switch string(data[:4]) {
			case "ANIM", "ANMF":
				return false
			case "VP8X":
				if n != 10 || data[8]&2 != 0 {
					return false
				}
			case "VP8L", "VP8 ":
				pictures++
			}
			data = data[8+padded:]
		}
		return pictures == 1
	case bytes.HasPrefix(body, []byte("\x89PNG\r\n\x1a\n")):
		data := body[8:]
		for len(data) >= 12 {
			n := int64(binary.BigEndian.Uint32(data))
			if n+12 > int64(len(data)) {
				return false
			}
			switch string(data[4:8]) {
			case "acTL", "fcTL", "fdAT":
				return false
			case "IEND":
				return n == 0 && len(data) == 12
			}
			data = data[12+n:]
		}
		return false
	case body[0] == 0xff && body[1] == 0xd8:
		return body[len(body)-2] == 0xff && body[len(body)-1] == 0xd9
	case string(body[4:8]) == "ftyp":
		return stillAVIF(body)
	}
	return false
}

func knownRaster(raw []byte) bool {
	return bytes.HasPrefix(raw, []byte("RIFF")) || bytes.HasPrefix(raw, []byte("\x89PNG")) ||
		bytes.HasPrefix(raw, []byte("\xff\xd8")) || (len(raw) >= 12 && string(raw[4:8]) == "ftyp")
}

func stillAVIF(body []byte) bool {
	seen := map[string]bool{}
	still := false
	for len(body) > 0 {
		if len(body) < 8 {
			return false
		}
		n := int64(binary.BigEndian.Uint32(body))
		kind := string(body[4:8])
		if n < 8 || n > int64(len(body)) || seen[kind] {
			return false
		}
		seen[kind] = true
		switch kind {
		case "ftyp":
			if n < 16 || n > 136 || n%4 != 0 {
				return false
			}
			for i := int64(8); i < n; i += 4 {
				if i == 12 {
					continue
				}
				switch string(body[i : i+4]) {
				case "avis":
					return false
				case "avif":
					still = true
				}
			}
		case "meta", "mdat", "free":
		default:
			return false
		}
		body = body[n:]
	}
	return still && seen["meta"] && seen["mdat"]
}

func decodeLogo(raw []byte) (img image.Image, err error) {
	defer func() {
		if recover() != nil {
			img, err = nil, storage.ErrSiteAsset
		}
	}()
	var config func(io.Reader) (image.Config, error)
	var decode func(io.Reader) (image.Image, error)
	switch {
	case bytes.HasPrefix(raw, []byte("RIFF")):
		config, decode = webp.DecodeConfig, webp.Decode
	case bytes.HasPrefix(raw, []byte("\x89PNG")):
		config, decode = png.DecodeConfig, png.Decode
	case string(raw[4:8]) == "ftyp":
		config = avif.DecodeConfig
		decode = func(r io.Reader) (image.Image, error) {
			return avif.Decode(r, avif.Options{FrameSizeLimit: imagePixelsLimit, MaxFrames: 1})
		}
	default:
		config, decode = jpeg.DecodeConfig, jpeg.Decode
	}
	shape, err := config(bytes.NewReader(raw))
	if err != nil || shape.Width < 1 || shape.Height < 1 || shape.Width > 8192 || shape.Height > 8192 ||
		int64(shape.Width)*int64(shape.Height) > imagePixelsLimit {
		return nil, storage.ErrSiteAsset
	}
	img, err = decode(bytes.NewReader(raw))
	if err != nil || img.Bounds().Dx() != shape.Width || img.Bounds().Dy() != shape.Height {
		return nil, storage.ErrSiteAsset
	}
	return img, nil
}

func encodeLogo(ctx context.Context, img image.Image) ([]byte, storage.SiteAsset, error) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	for side := min(max(w, h), 1024); side >= 1; side = side * 3 / 4 {
		if ctx.Err() != nil {
			return nil, storage.SiteAsset{}, ctx.Err()
		}
		rw, rh := max(1, side*w/max(w, h)), max(1, side*h/max(w, h))
		x, y := (side-rw)/2, (side-rh)/2
		canvas := image.NewNRGBA(image.Rect(0, 0, side, side))
		draw.CatmullRom.Scale(canvas, image.Rect(x, y, x+rw, y+rh), img, img.Bounds(), draw.Src, nil)
		var output bytes.Buffer
		if err := nativewebp.Encode(&output, canvas, nil); err != nil {
			return nil, storage.SiteAsset{}, err
		}
		if output.Len() > imageOutputLimit {
			continue
		}
		var validate *storage.Store
		asset, err := validate.ImportSiteAsset(ctx, storage.SiteLogo, bytes.NewReader(output.Bytes()), true)
		if err != nil {
			return nil, storage.SiteAsset{}, err
		}
		decoded, err := webp.Decode(bytes.NewReader(output.Bytes()))
		if err != nil || !sameAlpha(canvas, decoded) {
			return nil, storage.SiteAsset{}, storage.ErrSiteAsset
		}
		return output.Bytes(), asset, nil
	}
	return nil, storage.SiteAsset{}, storage.ErrSiteAsset
}

func sameAlpha(a, b image.Image) bool {
	for y := range a.Bounds().Dy() {
		for x := range a.Bounds().Dx() {
			_, _, _, aa := a.At(x, y).RGBA()
			_, _, _, ba := b.At(x, y).RGBA()
			if aa != ba {
				return false
			}
		}
	}
	return true
}
