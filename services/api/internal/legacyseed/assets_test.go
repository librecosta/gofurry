package legacyseed

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/librecosta/gofurry/services/api/internal/storage"
	"golang.org/x/image/webp"
)

func samplePNG(t testing.TB) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 96, 40))
	for y := range 40 {
		for x := range 96 {
			img.SetNRGBA(x, y, color.NRGBA{R: 210, G: uint8(x), B: 50, A: 128})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func makeObject(t testing.TB, root, key string, data []byte) {
	t.Helper()
	p := filepath.Join(root, key)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
}

type memoryObjects map[string][]byte

func (memoryObjects) row(string, any) error { return nil }
func (m memoryObjects) blob(path string, data []byte) error {
	m[path] = data
	return nil
}

func TestLogoFormatAndAlpha(t *testing.T) {
	ctx := context.Background()
	input := t.TempDir()
	root, err := privateRoot(input, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	dst := memoryObjects{}
	write := func(id, ext string, raw []byte) logo {
		t.Helper()
		key := "nav/sites/" + id + "/icon/" + sha(raw)[:32] + "." + ext
		makeObject(t, input, key, raw)
		obj, err := convertObject(ctx, root, key, dst, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		return obj
	}
	obj := write("1", "png", samplePNG(t))
	if obj.Status != "ready" || obj.Conversion != "lossless_webp_centered" || obj.Width != 96 || obj.Height != 96 ||
		obj.SHA != sha(dst[obj.Path]) {
		t.Fatalf("bad Logo projection: %+v", obj)
	}
	got, err := webp.Decode(bytes.NewReader(dst[obj.Path]))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, edge := got.At(48, 0).RGBA()
	_, _, _, center := got.At(48, 48).RGBA()
	_, _, _, lastEdge := got.At(48, 95).RGBA()
	if edge != 0 || lastEdge != 0 || center != 0x8080 {
		t.Fatalf("alpha %x %x %x", edge, center, lastEdge)
	}
	original := write("2", "webp", dst[obj.Path])
	if original.Status != "ready" || original.Conversion != "original_webp" || original.SHA != obj.SHA {
		t.Fatal("valid WebP was re-encoded")
	}
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 32, 24)), nil); err != nil {
		t.Fatal(err)
	}
	if got := write("3", "jpg", jpg.Bytes()); got.Status != "ready" || got.Width != 32 {
		t.Fatal("JPEG was not converted")
	}
	broken := append(bytes.Clone(samplePNG(t)), 1)
	if got := write("4", "png", broken); got.Status != "invalid_image" {
		t.Fatal("PNG with trailing bytes was accepted")
	}
	if got := write("5", "svg", []byte("<svg xmlns='http://www.w3.org/2000/svg'><script>fixture</script></svg>")); got.Status != "unsupported_image_format" {
		t.Fatal("untrusted SVG entered the candidate package")
	}
	animated := append(bytes.Clone(samplePNG(t)[:33]), []byte{0, 0, 0, 0, 'a', 'c', 'T', 'L', 0, 0, 0, 0}...)
	animated = append(animated, samplePNG(t)[33:]...)
	if validImageEnvelope(animated) {
		t.Fatal("animated PNG was accepted")
	}
	avifData, err := os.ReadFile("../storage/testdata/site-cover.avif")
	if err != nil {
		t.Fatal(err)
	}
	if got := write("6", "avif", avifData); got.Status != "ready" {
		t.Fatalf("static AVIF was not converted: %v", got.Status)
	}
	key := "nav/sites/7/icon/" + strings.Repeat("a", 32) + ".png"
	makeObject(t, input, key, samplePNG(t))
	if got, err := convertObject(ctx, root, key, dst, "", nil); err != nil || got.Status != "source_checksum_mismatch" {
		t.Fatal("wrong bytes under a legacy digest were accepted")
	}
	var store *storage.Store
	if _, err := store.ImportSiteAsset(ctx, storage.SiteLogo, bytes.NewReader(dst[obj.Path]), true); err != nil {
		t.Fatal(err)
	}
}

func TestLogoMirrorDoesNotFollowLinks(t *testing.T) {
	input := t.TempDir()
	outside := t.TempDir()
	raw := samplePNG(t)
	key := "nav/sites/1/icon/" + sha(raw)[:32] + ".png"
	makeObject(t, outside, filepath.Base(key), raw)
	if err := os.MkdirAll(filepath.Join(input, filepath.Dir(key)), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, filepath.Base(key)), filepath.Join(input, key)); err != nil {
		t.Skip("host does not permit creating a symbolic link")
	}
	root, err := privateRoot(input, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if obj, err := convertObject(context.Background(), root, key, memoryObjects{}, "", nil); err != nil || obj.Status != "object_read_rejected" {
		t.Fatalf("linked object not rejected: %v %v", obj.Status, err)
	}
}

func TestPrivateConfigAndUnsafeCandidates(t *testing.T) {
	for _, input := range []string{"http://localhost", "http://127.0.0.1", "https://100.64.0.25", "https://192.168.0.1",
		"http://[::ffff:127.0.0.1]", "http://①②⑦.⓪.⓪.①", "http://0x7f000001", "https://example.com/%2e%2e",
		"https://example.com/a%2Fb", "https://example.com/%00", "http://name:pw@example.com", "https://example.com/?key=secret",
		"https://example.com/#login", "https://example.com/a\\b", "javascript:confirm(1)", "file:///private", "https://example.com:99999"} {
		if u := cleanURL(input); u != "" {
			t.Errorf("unsafe URL was accepted: %s", input)
		}
	}
	if got := cleanURL(" HTTPS://中文.example:443/a "); got != "https://xn--fiq228c.example/a" {
		t.Errorf("unexpected IDNA URL %q", got)
	}

	path := filepath.Join(t.TempDir(), "spec.yaml")
	for _, body := range []string{"snapshot_id: ./escape\npassword: no-leak", "snapshot_id: wrong\n---\nhidden: no-leak",
		"gfn: {host: remote.example, password: no-leak,\n", strings.Repeat("secret", 10000)} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadConfig(path, ""); err == nil || strings.Contains(err.Error(), "no-leak") {
			t.Fatal("private YAML validation failed")
		}
	}
}

func BenchmarkLogoConversion(b *testing.B) {
	img, err := decodeLogo(samplePNG(b))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	ctx := context.Background()
	for b.Loop() {
		if _, _, err := encodeLogo(ctx, img); err != nil {
			b.Fatal(err)
		}
	}
}
