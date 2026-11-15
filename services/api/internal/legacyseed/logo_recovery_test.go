package legacyseed

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="80" height="40" viewBox="0 0 80 40"><defs><linearGradient id="color"><stop offset="0" stop-color="#d30"/><stop offset="1" stop-color="#0c5"/></linearGradient></defs><rect x="4" y="4" width="72" height="32" fill="url(#color)" opacity=".8"/></svg>`

func TestStaticSVGProfile(t *testing.T) {
	w3c := `<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">`
	doc, err := validateSVG([]byte("<?xml version=\"1.0\"?>\n" + w3c + fixtureSVG))
	if err != nil || bytes.Contains(doc, []byte("DOCTYPE")) || !bytes.HasSuffix(doc, []byte(fixtureSVG)) {
		t.Fatal("legacy static SVG was not stripped and accepted")
	}
	for name, doc := range map[string]string{
		"entity":                 `<!DOCTYPE svg [<!ENTITY local SYSTEM "file:///private">]>` + fixtureSVG,
		"external_doctype":       `<!DOCTYPE svg SYSTEM "file:///private">` + fixtureSVG,
		"processing_instruction": `<?xml-stylesheet href="https://example.com/style.css"?>` + fixtureSVG,
		"script":                 insertSVG(`<script>fixture</script>`),
		"event":                  insertSVG(`<circle onload="fixture" r="8"/>`),
		"image":                  insertSVG(`<image href="file:///private"/>`),
		"filter_image":           insertSVG(`<filter><feImage href="http://example.com/img"/></filter>`),
		"relative_paint":         insertSVG(`<rect fill="url(private.svg)"/>`),
		"relative_fragment":      insertSVG(`<linearGradient href="private.svg#color"/>`),
		"untrusted_css":          insertSVG(`<style>@import "local.css";</style>`),
		"css_escape":             insertSVG(`<style>.mark {fill: u\72l(local)}</style>`),
		"split_css":              insertSVG(`<style>.mark {fill: u<!-- -->rl(local)}</style>`),
		"base_url":               insertSVG(`<g xml:base="file:///private" />`),
		"wrong_root":             `<html><svg/></html>`,
		"second_root":            fixtureSVG + fixtureSVG,
		"nesting":                insertSVG(strings.Repeat("<g>", 25) + strings.Repeat("</g>", 25)),
		"viewport":               strings.ReplaceAll(fixtureSVG, "80", "800000"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := validateSVG([]byte(doc)); err == nil {
				t.Fatal("unsafe SVG reached rendering")
			}
		})
	}
}

func insertSVG(body string) string { return strings.Replace(fixtureSVG, "</svg>", body+"</svg>", 1) }

func TestRecoveredLogoRendering(t *testing.T) {
	renderer := os.Getenv("GOFURRY_TEST_RESVG")
	if renderer == "" {
		t.Skip("set GOFURRY_TEST_RESVG to the absolute path of the isolated resvg executable")
	}
	ctx := context.Background()
	dst := memoryObjects{}
	row, err := recoverLogo(ctx, []byte(fixtureSVG), logo{}, dst, renderer)
	if err != nil || row.Status != "ready" || row.Conversion != "svg_webp_centered" || row.Width != 512 || row.Height != 512 {
		t.Fatalf("static SVG conversion failed: %s, %v", row.Status, err)
	}
	img, err := decodeLogo(dst[row.Path])
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, edge := img.At(256, 96).RGBA()
	_, _, _, center := img.At(256, 256).RGBA()
	if edge != 0 || center < 50*257 || center >= 0xffff {
		t.Fatal("aspect ratio or SVG alpha was lost")
	}
}

func TestICOFrameSelectionAndQuality(t *testing.T) {
	small := icoPNG(t, 16, true)
	large := icoPNG(t, 48, true)
	blank := icoPNG(t, 64, false)
	raw := iconFrames([]int{16, 48, 64}, [][]byte{small, large, blank})
	img := decodeICO(raw)
	if img == nil || img.Bounds().Dx() != 48 {
		t.Fatal("did not select the largest usable frame")
	}
	_, _, _, edge := img.At(0, 0).RGBA()
	_, _, _, center := img.At(24, 24).RGBA()
	if edge != 0 || center != 192*257 {
		t.Fatal("ICO alpha was lost")
	}
	corrupt := bytes.Clone(raw)
	binary.LittleEndian.PutUint32(corrupt[34:], ^uint32(0))
	if fallback := decodeICO(corrupt); fallback == nil || fallback.Bounds().Dx() != 16 {
		t.Fatal("did not fall back from a corrupt frame")
	}
	ctx := context.Background()
	d, err := recoverLogo(ctx, iconFrames([]int{64}, [][]byte{blank}), logo{}, memoryObjects{}, "")
	if err != nil || d.Status != "logo_quality_rejected" {
		t.Fatalf("blank frame became a Logo: %s, %v", d.Status, err)
	}
}

func TestSharedLegacyLogoNeedsAttribution(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(fixtureSVG)
	digest := sha(raw)[:32]
	key := "nav/sites/1/icon/" + digest + ".svg"
	makeObject(t, dir, key, raw)
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	dst := memoryObjects{}
	row, err := convertObject(context.Background(), root, key, dst, filepath.Join(dir, "unused"), map[string]int{digest: 3})
	if err != nil || row.Status != "logo_suspected_placeholder" || len(dst) != 0 {
		t.Fatal("shared SVG was treated as an attributable brand")
	}
}

func icoPNG(t *testing.T, side int, visible bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	if visible {
		for y := side / 4; y < side*3/4; y++ {
			for x := side / 4; x < side*3/4; x++ {
				img.SetNRGBA(x, y, color.NRGBA{R: 64, G: 190, A: 192})
			}
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func iconFrames(sides []int, pngs [][]byte) []byte {
	body := make([]byte, 6+16*len(sides))
	body[2] = 1
	binary.LittleEndian.PutUint16(body[4:], uint16(len(sides)))
	for i, side := range sides {
		entry := body[6+16*i:]
		entry[0], entry[1] = byte(side), byte(side)
		binary.LittleEndian.PutUint32(entry[8:], uint32(len(pngs[i])))
		binary.LittleEndian.PutUint32(entry[12:], uint32(len(body)))
		body = append(body, pngs[i]...)
	}
	return body
}
