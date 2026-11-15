package legacyseed

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"image"
	"image/png"
	"io"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const svgNamespace = "http://www.w3.org/2000/svg"

var (
	svgDoctype    = regexp.MustCompile(`^DOCTYPE\s+svg\s+PUBLIC\s+["']-//W3C//DTD SVG [0-9. A-Za-z]+//EN["']\s+["']https?://www\.w3\.org/[A-Za-z0-9./_-]+\.dtd["']\s*$`)
	svgFragment   = regexp.MustCompile(`^#[A-Za-z_][A-Za-z0-9_.:-]{0,127}$`)
	svgLocalPaint = regexp.MustCompile(`(?i)url\(\s*#[A-Za-z_][A-Za-z0-9_.:-]{0,127}\s*\)`)
	svgLength     = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)(px|pt|in|mm|cm|pc|%)?$`)
	svgCSS        = regexp.MustCompile(`^[a-zA-Z0-9#.,%():;_+* \t\r\n{}>\-]*$`)
	svgErr        = errors.New("SVG outside the legacy Logo profile")
)

func validateSVG(raw []byte) ([]byte, error) {
	if len(raw) > 128<<10 {
		return nil, svgErr
	}
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	d := xml.NewDecoder(bytes.NewReader(raw))
	var rootSeen, doctype, declaration bool
	var stack []string
	var style strings.Builder
	var doctypeStart, doctypeEnd int64
	nodes, filters, primitives := 0, 0, 0
	for {
		start := d.InputOffset()
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, svgErr
		}
		switch item := token.(type) {
		case xml.StartElement:
			nodes++
			if item.Name.Local == "filter" {
				filters++
			}
			if strings.HasPrefix(item.Name.Local, "fe") {
				primitives++
			}
			if nodes > 512 || len(stack) >= 24 || filters > 4 || primitives > 24 ||
				item.Name.Space != "" && item.Name.Space != svgNamespace ||
				!safeSVGElement(item.Name.Local) || len(item.Attr) > 48 {
				return nil, svgErr
			}
			if len(stack) == 0 {
				if rootSeen || item.Name.Local != "svg" || !svgViewportOK(item.Attr) {
					return nil, svgErr
				}
				rootSeen = true
			} else if item.Name.Local == "svg" || stack[len(stack)-1] == "style" {
				return nil, svgErr
			}
			seen := map[xml.Name]bool{}
			for _, attr := range item.Attr {
				if seen[attr.Name] || !safeSVGAttribute(item.Name.Local, attr) {
					return nil, svgErr
				}
				seen[attr.Name] = true
			}
			stack = append(stack, item.Name.Local)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, svgErr
			}
			if stack[len(stack)-1] == "style" {
				if !safeSVGStyle(style.String()) {
					return nil, svgErr
				}
				style.Reset()
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 && stack[len(stack)-1] == "style" {
				if style.Len()+len(item) > 4096 {
					return nil, svgErr
				}
				style.Write(item)
				continue
			}
			if len(bytes.TrimSpace(item)) != 0 {
				if len(stack) == 0 {
					return nil, svgErr
				}
				switch stack[len(stack)-1] {
				case "metadata", "title", "desc":
				default:
					return nil, svgErr
				}
			}
		case xml.ProcInst:
			if item.Target != "xml" || start != 0 || declaration {
				return nil, svgErr
			}
			declaration = true
		case xml.Directive:
			// Remove a recognized legacy W3C declaration. No doctype or
			// entity declaration reaches the renderer.
			if rootSeen || doctype || !svgDoctype.Match(item) {
				return nil, svgErr
			}
			doctype = true
			doctypeStart, doctypeEnd = start, d.InputOffset()
		}
	}
	if !rootSeen || len(stack) != 0 {
		return nil, svgErr
	}
	if doctype {
		raw = append(bytes.Clone(raw[:doctypeStart]), raw[doctypeEnd:]...)
	}
	return raw, nil
}

func safeSVGElement(name string) bool {
	switch name {
	case "svg", "g", "defs", "metadata", "title", "desc", "style", "path", "circle", "ellipse", "line", "rect", "polyline", "polygon",
		"linearGradient", "radialGradient", "stop", "clipPath", "filter", "feColorMatrix", "feComposite", "feFlood",
		"feGaussianBlur", "feOffset", "feMerge", "feMergeNode":
		return true
	}
	return false
}

func safeSVGAttribute(element string, attr xml.Attr) bool {
	name, value := attr.Name.Local, attr.Value
	switch attr.Name.Space {
	case "xmlns":
		return len(value) <= 256
	case "http://www.w3.org/XML/1998/namespace":
		return name == "space" && (value == "default" || value == "preserve")
	case "", "http://www.w3.org/1999/xlink":
	default:
		return false
	}
	if name == "href" {
		return (element == "linearGradient" || element == "radialGradient") && svgFragment.MatchString(value)
	}
	if attr.Name.Space != "" {
		return false
	}
	switch name {
	case "xmlns":
		return value == svgNamespace
	case "d", "points":
		return len(value) <= 48<<10 && !strings.ContainsAny(value, "/\\&")
	case "style":
		return safeSVGStyle(value)
	case "fill", "stroke", "filter", "clip-path", "stop-color", "flood-color":
		return len(value) <= 256 && safeSVGStyle(value)
	case "stdDeviation":
		parts := strings.Fields(strings.ReplaceAll(value, ",", " "))
		if len(parts) == 0 || len(parts) > 2 {
			return false
		}
		for _, part := range parts {
			v, err := strconv.ParseFloat(part, 64)
			if err != nil || math.IsNaN(v) || v < 0 || v > 128 {
				return false
			}
		}
		return true
	case "id", "class", "version", "data-name", "data-hk":
		return len(value) <= 256
	case "width", "height", "viewBox", "preserveAspectRatio", "x", "y", "x1", "x2", "y1", "y2", "cx", "cy", "r", "rx", "ry",
		"fx", "fy", "fr", "opacity", "fill-opacity", "fill-rule", "stroke-width", "stroke-opacity", "stroke-linecap",
		"stroke-linejoin", "stroke-miterlimit", "offset", "stop-opacity", "transform", "gradientTransform", "gradientUnits",
		"spreadMethod", "clipPathUnits", "clip-rule", "filterUnits", "primitiveUnits", "dx", "dy", "type", "values",
		"result", "in", "in2", "operator", "flood-opacity", "k1", "k2", "k3", "k4", "color-interpolation-filters":
		return len(value) <= 1024 && safeSVGStyle(value)
	}
	return false
}

func safeSVGStyle(value string) bool {
	// CSS escapes, comments, at-rules, resource schemes and every URL
	// other than a plain local fragment are outside this profile.
	rest := svgLocalPaint.ReplaceAllString(value, "")
	return len(value) <= 4096 && svgCSS.MatchString(rest) && !strings.Contains(strings.ToLower(rest), "url")
}

func svgViewportOK(attrs []xml.Attr) bool {
	var width, height float64
	var viewW, viewH float64
	for _, attr := range attrs {
		if attr.Name.Space != "" {
			continue
		}
		switch attr.Name.Local {
		case "width":
			width = svgPixels(attr.Value)
		case "height":
			height = svgPixels(attr.Value)
		case "viewBox":
			parts := strings.Fields(strings.ReplaceAll(attr.Value, ",", " "))
			if len(parts) != 4 {
				return false
			}
			for i, p := range parts {
				n, err := strconv.ParseFloat(p, 64)
				if err != nil || math.IsNaN(n) || math.Abs(n) > 16384 || i >= 2 && n < .01 {
					return false
				}
				if i == 2 {
					viewW = n
				} else if i == 3 {
					viewH = n
				}
			}
		}
	}
	if width == 0 || height == 0 {
		width, height = viewW, viewH
	}
	return width >= .01 && height >= .01 && min(width, height)/max(width, height) >= 1.0/64
}

func svgPixels(value string) float64 {
	parts := svgLength.FindStringSubmatch(strings.TrimSpace(value))
	if len(parts) != 3 {
		return 0
	}
	n, err := strconv.ParseFloat(parts[1], 64)
	if err != nil || n > 16384 {
		return 0
	}
	switch parts[2] {
	case "pt":
		return n * 4 / 3
	case "in":
		return n * 96
	case "mm":
		return n * 96 / 25.4
	case "cm":
		return n * 96 / 2.54
	case "pc":
		return n * 16
	case "%":
		return 0
	}
	return n
}

func renderSVG(ctx context.Context, renderer string, safe []byte) image.Image {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	output := &limitedPNG{}
	cmd := exec.CommandContext(ctx, renderer, "--quiet", "--skip-system-fonts", "--width", "512", "--height", "512", "-", "-c")
	cmd.Env = []string{}
	cmd.Stdin = bytes.NewReader(safe)
	cmd.Stdout = output
	cmd.WaitDelay = 200 * time.Millisecond
	if cmd.Run() != nil {
		return nil
	}
	body := output.Bytes()
	if !validImageEnvelope(body) {
		return nil
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(body))
	if err != nil || min(cfg.Width, cfg.Height) < 1 || max(cfg.Width, cfg.Height) != 512 {
		return nil
	}
	img, err := png.Decode(bytes.NewReader(body))
	if err != nil {
		return nil
	}
	return img
}

type limitedPNG struct{ bytes.Buffer }

func (b *limitedPNG) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 2<<20 {
		return 0, svgErr
	}
	return b.Buffer.Write(p)
}
