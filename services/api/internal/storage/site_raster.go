package storage

import "encoding/binary"

func siteWebPEnvelope(raw []byte) bool {
	if string(raw[:4]) != "RIFF" || string(raw[8:12]) != "WEBP" || uint64(binary.LittleEndian.Uint32(raw[4:8]))+8 != uint64(len(raw)) {
		return false
	}
	seen := map[string]bool{}
	coded := 0
	for chunk := raw[12:]; len(chunk) > 0; {
		if len(chunk) < 8 {
			return false
		}
		kind := string(chunk[:4])
		size := uint64(binary.LittleEndian.Uint32(chunk[4:8]))
		end := 8 + size + (size & 1)
		if size == 0 || end > uint64(len(chunk)) || seen[kind] {
			return false
		}
		seen[kind] = true
		switch kind {
		case "VP8 ", "VP8L":
			coded++
		case "VP8X":
			if size != 10 || chunk[8]&0xcf != 0 {
				return false
			}
		case "ALPH", "ICCP":
		default:
			return false
		}
		chunk = chunk[end:]
	}
	return coded == 1
}

// The decoder checks the image payload. These container checks admit a still
// image and reject metadata items, extra top-level payloads and sequences.
func siteAVIFEnvelope(raw []byte) bool {
	seen := map[string]bool{}
	ok := walkSiteBoxes(raw, func(kind string, body []byte) bool {
		if seen[kind] {
			return false
		}
		seen[kind] = true
		switch kind {
		case "ftyp":
			if len(body) < 8 || len(body) > 128 || len(body)%4 != 0 {
				return false
			}
			still := false
			for i := 0; i < len(body); i += 4 {
				if i == 4 {
					continue
				}
				switch string(body[i : i+4]) {
				case "avis":
					return false
				case "avif":
					still = true
				}
			}
			return still
		case "meta":
			if len(body) < 4 || len(body) > 1<<20 {
				return false
			}
			return walkSiteBoxes(body[4:], func(kind string, body []byte) bool {
				switch kind {
				case "hdlr", "pitm", "iloc", "iprp", "iref", "idat", "grpl":
					return true
				case "iinf":
					if len(body) < 6 {
						return false
					}
					offset := 6
					switch body[0] {
					case 0:
					case 1:
						offset = 8
					default:
						return false
					}
					if len(body) <= offset {
						return false
					}
					return walkSiteBoxes(body[offset:], func(kind string, body []byte) bool {
						if kind != "infe" || len(body) < 12 {
							return false
						}
						offset := 8
						switch body[0] {
						case 2:
						case 3:
							offset = 10
						default:
							return false
						}
						if len(body) < offset+4 {
							return false
						}
						item := string(body[offset : offset+4])
						return item == "av01" || item == "grid"
					})
				default:
					return false
				}
			})
		case "mdat":
			return len(body) > 0
		case "free":
			if len(body) > 1024 {
				return false
			}
			for _, b := range body {
				if b != 0 {
					return false
				}
			}
			return true
		default:
			return false
		}
	})
	return ok && seen["ftyp"] && seen["meta"] && seen["mdat"]
}

func walkSiteBoxes(raw []byte, visit func(string, []byte) bool) bool {
	for count := 0; len(raw) > 0; count++ {
		if count == 4096 || len(raw) < 8 {
			return false
		}
		size := uint64(binary.BigEndian.Uint32(raw[:4]))
		header := uint64(8)
		switch size {
		case 0:
			size = uint64(len(raw))
		case 1:
			if len(raw) < 16 {
				return false
			}
			header, size = 16, binary.BigEndian.Uint64(raw[8:16])
		}
		if size < header || size > uint64(len(raw)) || !visit(string(raw[4:8]), raw[header:size]) {
			return false
		}
		raw = raw[size:]
	}
	return true
}
