// Package geoip resolves approximate cities from one local DB-IP Lite City MMDB.
package geoip

import (
	"fmt"
	"math"
	"net/netip"
	"slices"
	"strings"
	"sync"

	"github.com/oschwald/maxminddb-golang/v2"
)

type Names struct {
	ZH string `json:"zh"`
	EN string `json:"en"`
}

type Location struct {
	City         Names   `json:"city"`
	CountryCode  string  `json:"country_code"`
	Subdivisions []Names `json:"subdivisions"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
}

// These are the fields observed in DBIP-City-Lite 2026-11. City/subdivision
// names are often English-only; there is no reliable city ID, postcode,
// timezone, accuracy radius or subdivision ISO code in this Lite dataset.
type namedRecord struct {
	Names map[string]string `maxminddb:"names"`
}

type cityRecord struct {
	City    namedRecord `maxminddb:"city"`
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
	Subdivisions []namedRecord `maxminddb:"subdivisions"`
	Location     struct {
		Latitude  *float64 `maxminddb:"latitude"`
		Longitude *float64 `maxminddb:"longitude"`
	} `maxminddb:"location"`
}

type Resolver struct {
	mu sync.RWMutex
	db *maxminddb.Reader
}

// Open is called once at startup. A blank path disables the feature. The caller
// can degrade on an error without exposing a file path or raw database error.
func Open(path string) (*Resolver, error) {
	if path == "" {
		return nil, nil
	}
	db, err := maxminddb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open city database: %w", err)
	}
	if db.Metadata.DatabaseType != "DBIP-City-Lite" {
		_ = db.Close()
		return nil, fmt.Errorf("expected DBIP-City-Lite database")
	}
	if err := db.Verify(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("verify city database: %w", err)
	}
	return &Resolver{db: db}, nil
}

// Close runs after draining. The lock also protects a lookup if shutdown times
// out, and makes repeated cleanup safe without closing a reader still in use.
func (r *Resolver) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.db == nil {
		return nil
	}
	err := r.db.Close()
	r.db = nil
	return err
}

func (r *Resolver) Resolve(ip netip.Addr) *Location {
	if r == nil || !publicIP(ip) {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.db == nil {
		return nil
	}
	result := r.db.Lookup(ip.Unmap())
	if !result.Found() {
		return nil
	}
	var record cityRecord
	if err := result.Decode(&record); err != nil {
		return nil
	}
	return locationFrom(record)
}

func locationFrom(record cityRecord) *Location {
	city := names(record.City.Names)
	lat, lon := record.Location.Latitude, record.Location.Longitude
	code := record.Country.ISOCode
	if city.EN == "" || len(code) != 2 || code[0] < 'A' || code[0] > 'Z' || code[1] < 'A' || code[1] > 'Z' ||
		lat == nil || lon == nil || math.IsNaN(*lat) || math.IsNaN(*lon) || math.IsInf(*lat, 0) || math.IsInf(*lon, 0) ||
		*lat < -90 || *lat > 90 || *lon < -180 || *lon > 180 {
		return nil
	}
	location := &Location{City: city, CountryCode: code, Subdivisions: []Names{},
		Latitude: math.Round(*lat*100) / 100, Longitude: math.Round(*lon*100) / 100}
	for _, subdivision := range record.Subdivisions {
		if name := names(subdivision.Names); name.EN != "" {
			location.Subdivisions = append(location.Subdivisions, name)
		}
	}
	return location
}

// Fallback only reuses names actually present in the record, never translates
// or guesses. Missing zh/en falls back to the other, then a stable available name.
func names(values map[string]string) Names {
	zh, en := strings.TrimSpace(values["zh-CN"]), strings.TrimSpace(values["en"])
	if zh == "" {
		zh = strings.TrimSpace(values["zh"])
	}
	if en == "" {
		en = zh
	}
	if en == "" {
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			if en = strings.TrimSpace(values[key]); en != "" {
				break
			}
		}
	}
	if zh == "" {
		zh = en
	}
	return Names{ZH: zh, EN: en}
}
