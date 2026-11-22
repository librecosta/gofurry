package catalog

import "strings"

// NewGameSlug derives a decorative path from the English title. Public ID
// distinguishes identities even when titles produce the same path.
func NewGameSlug(texts []Localization) string {
	return englishTitleSlug(texts, "game")
}

func englishTitleSlug(texts []Localization, fallback string) string {
	for _, text := range texts {
		if text.Locale != "en" {
			continue
		}
		words := strings.FieldsFunc(strings.ToLower(text.Name), func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
		})
		path := strings.Join(words, "-")
		if path != "" {
			return strings.TrimRight(path[:min(len(path), 80)], "-")
		}
	}
	return fallback
}
