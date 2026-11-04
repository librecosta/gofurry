// Package cities reads one optional, immutable GeoNames weather-city catalog.
package cities

import (
	"encoding/json/v2"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/librecosta/gofurry/services/api/internal/geoip"
)

type City struct {
	ID          string      `json:"city_id"`
	Name        string      `json:"name"`
	Names       geoip.Names `json:"names"`
	CountryCode string      `json:"country_code"`
	Admin       string      `json:"admin"`
}

type entry struct {
	City
	Aliases    []string `json:"aliases"`
	Latitude   *float64 `json:"latitude"`
	Longitude  *float64 `json:"longitude"`
	Population int64    `json:"population"`
	terms      []string
}

type Catalog struct {
	entries []entry
	byID    map[string]int
}

func ValidID(id string) bool {
	if len(id) == 0 || len(id) > 12 || id[0] == '0' {
		return false
	}
	for _, ch := range id {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func ValidQuery(q string) bool {
	if !utf8.ValidString(q) || len(q) > 320 || utf8.RuneCountInString(q) > 80 {
		return false
	}
	for _, ch := range q {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && !unicode.IsMark(ch) && ch != ' ' && !strings.ContainsRune("'-.,’", ch) {
			return false
		}
	}
	return true
}

func Open(path string) (*Catalog, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open city catalog")
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || stat.Size() > 64<<20 {
		return nil, fmt.Errorf("city catalog exceeds bound")
	}
	var data struct {
		Format int     `json:"format"`
		Source string  `json:"source"`
		Cities []entry `json:"cities"`
	}
	if err = json.UnmarshalRead(io.LimitReader(f, (64<<20)+1), &data); err != nil || data.Format != 1 || data.Source != "GeoNames cities5000" || len(data.Cities) == 0 || len(data.Cities) > 100000 {
		return nil, fmt.Errorf("invalid city catalog")
	}
	c := &Catalog{entries: data.Cities, byID: make(map[string]int, len(data.Cities))}
	for i := range c.entries {
		e := &c.entries[i]
		_, duplicate := c.byID[e.ID]
		if !ValidID(e.ID) || duplicate || !validText(e.Name) || !validText(e.Names.ZH) || !validText(e.Names.EN) || len(e.Admin) > 256 ||
			len(e.CountryCode) != 2 || strings.Trim(e.CountryCode, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" || len(e.Aliases) > 512 ||
			e.Latitude == nil || e.Longitude == nil || math.IsNaN(*e.Latitude) || math.IsNaN(*e.Longitude) || math.IsInf(*e.Latitude, 0) || math.IsInf(*e.Longitude, 0) || math.Abs(*e.Latitude) > 90 || math.Abs(*e.Longitude) > 180 {
			return nil, fmt.Errorf("invalid city entry")
		}
		c.byID[e.ID] = i
		for _, name := range append(e.Aliases, e.Name, e.Names.ZH, e.Names.EN) {
			if !validText(name) {
				return nil, fmt.Errorf("invalid city alias")
			}
			e.terms = append(e.terms, strings.ToLower(name))
		}
	}
	return c, nil
}

func validText(s string) bool {
	return s != "" && utf8.ValidString(s) && utf8.RuneCountInString(s) <= 256 && strings.IndexFunc(s, unicode.IsControl) < 0
}

func (c *Catalog) Resolve(id string) *geoip.Location {
	if c == nil {
		return nil
	}
	i, ok := c.byID[id]
	if !ok {
		return nil
	}
	e := c.entries[i]
	return &geoip.Location{City: e.Names, CountryCode: e.CountryCode, Subdivisions: []geoip.Names{{ZH: e.Admin, EN: e.Admin}},
		Latitude: math.Round(*e.Latitude*100) / 100, Longitude: math.Round(*e.Longitude*100) / 100}
}

// A bounded local scan avoids a second search service or an unbounded query cache.
// Exact/prefix names rank ahead of substrings; population resolves equal matches.
func (c *Catalog) Search(q string) []City {
	result := []City{}
	q = strings.ToLower(strings.TrimSpace(q))
	if c == nil || q == "" || !ValidQuery(q) {
		return result
	}
	type match struct{ index, score int }
	best := make([]match, 0, 11)
	for i, e := range c.entries {
		score := 4
		for _, term := range e.terms {
			switch {
			case term == q:
				score = 0
			case strings.HasPrefix(term, q):
				score = min(score, 1)
			case strings.Contains(term, q):
				score = min(score, 2)
			}
			if score == 0 {
				break
			}
		}
		if score == 4 {
			continue
		}
		best = append(best, match{i, score})
		slices.SortFunc(best, func(a, b match) int {
			if a.score != b.score {
				return a.score - b.score
			}
			x, y := c.entries[a.index], c.entries[b.index]
			if x.Population > y.Population {
				return -1
			}
			if x.Population < y.Population {
				return 1
			}
			return strings.Compare(x.ID, y.ID)
		})
		if len(best) > 10 {
			best = best[:10]
		}
	}
	for _, item := range best {
		result = append(result, c.entries[item.index].City)
	}
	return result
}
