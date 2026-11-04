package geoip

import (
	"math"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPublicIP(t *testing.T) {
	for _, address := range []string{"8.8.8.8", "223.5.5.5", "2001:4860:4860::8888", "2400:3200::1", "::ffff:8.8.8.8"} {
		if !publicIP(netip.MustParseAddr(address)) {
			t.Errorf("rejected public address %s", address)
		}
	}
	for _, address := range []string{"", "not-an-ip", "8.8.8.8:53", "0.0.0.0", "0.1.2.3", "10.2.3.4", "100.64.0.1", "127.0.0.1", "169.254.1.2", "172.16.0.1", "192.168.1.2", "192.0.0.1", "192.0.2.1", "192.88.99.1", "198.18.0.1", "198.51.100.0", "198.51.100.1", "203.0.113.1", "224.0.0.1", "240.0.0.1", "255.255.255.255", "::", "::1", "fc00::1", "fe80::1", "ff02::1", "::ffff:127.0.0.1", "::ffff:192.168.1.2", "64:ff9b::808:808", "100::1", "2001::1", "2001:2::1", "2001:db8::1", "2002:808:808::1", "3ffe::1", "3fff::1", "5f00::1", "2001:4860::1%eth0"} {
		ip, _ := netip.ParseAddr(address)
		if publicIP(ip) {
			t.Errorf("accepted invalid/special address %s", address)
		}
	}
}

func TestNamesFallback(t *testing.T) {
	for _, tc := range []struct {
		values map[string]string
		want   Names
	}{
		{map[string]string{"en": "Chengdu", "zh-CN": "成都"}, Names{ZH: "成都", EN: "Chengdu"}},
		{map[string]string{"en": "Hangzhou"}, Names{ZH: "Hangzhou", EN: "Hangzhou"}},
		{map[string]string{"zh": "成都"}, Names{ZH: "成都", EN: "成都"}},
		{map[string]string{"en": " ", "zh-CN": "", "fr": " Paris ", "de": "Paris"}, Names{ZH: "Paris", EN: "Paris"}},
		{nil, Names{}},
	} {
		if got := names(tc.values); got != tc.want {
			t.Errorf("names = %+v, want %+v", got, tc.want)
		}
	}
}

func TestLocationRequiresCityAndCoordinates(t *testing.T) {
	valid := func() cityRecord {
		var r cityRecord
		r.City.Names = map[string]string{"en": "Hangzhou"}
		r.Country.ISOCode = "CN"
		lat, lon := 30.2742, 120.155
		r.Location.Latitude, r.Location.Longitude = &lat, &lon
		r.Subdivisions = []namedRecord{{Names: map[string]string{"en": "Zhejiang"}}}
		return r
	}
	want := &Location{City: Names{ZH: "Hangzhou", EN: "Hangzhou"}, CountryCode: "CN", Subdivisions: []Names{{ZH: "Zhejiang", EN: "Zhejiang"}}, Latitude: 30.27, Longitude: 120.16}
	if got := locationFrom(valid()); !reflect.DeepEqual(got, want) {
		t.Fatalf("location = %+v", got)
	}
	for _, mutate := range []func(*cityRecord){
		func(r *cityRecord) { r.City.Names = nil },
		func(r *cityRecord) { r.Country.ISOCode = "" },
		func(r *cityRecord) { r.Country.ISOCode = "invalid" },
		func(r *cityRecord) { r.Location.Latitude = nil },
		func(r *cityRecord) { r.Location.Longitude = nil },
		func(r *cityRecord) { *r.Location.Latitude = 91 },
		func(r *cityRecord) { *r.Location.Longitude = -181 },
		func(r *cityRecord) { *r.Location.Latitude = math.NaN() },
		func(r *cityRecord) { *r.Location.Longitude = math.Inf(1) },
	} {
		r := valid()
		mutate(&r)
		if locationFrom(r) != nil {
			t.Fatal("incomplete/invalid city was guessed")
		}
	}
	r := valid()
	*r.Location.Latitude = 0
	*r.Location.Longitude = 0
	r.Subdivisions = nil
	if got := locationFrom(r); got == nil || got.Subdivisions == nil {
		t.Fatal("valid zero coordinates or absent subdivisions rejected")
	}
}

func TestDatabaseDisabledMissingCorrupt(t *testing.T) {
	resolver, err := Open("")
	if err != nil || resolver != nil || resolver.Resolve(netip.MustParseAddr("8.8.8.8")) != nil || resolver.Close() != nil {
		t.Fatal("disabled resolver contract")
	}
	path := filepath.Join(t.TempDir(), "city.mmdb")
	if _, err := Open(path); err == nil {
		t.Fatal("missing database accepted")
	}
	if err := os.WriteFile(path, []byte("not a database"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("corrupt database accepted")
	}
}

func TestLocalDBIPCity(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "data", "geoip", "dbip-city-lite-2026-11.mmdb")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("local November 2026 MMDB not installed; see docs/v0.1-c1-weather.md")
	}
	resolver, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	for _, tc := range []struct{ address, city, country string }{
		{"8.8.8.8", "Mountain View", "US"},
		{"223.5.5.5", "Hangzhou", "CN"},
		{"2001:4860:4860::8888", "Montreal", "CA"},
		{"2400:3200::1", "Hangzhou", "CN"},
		{"::ffff:223.5.5.5", "Hangzhou", "CN"},
	} {
		got := resolver.Resolve(netip.MustParseAddr(tc.address))
		if got == nil || got.City.EN != tc.city || got.CountryCode != tc.country {
			t.Fatalf("%s: %+v", tc.address, got)
		}
		t.Logf("public test fixture %s: %+v", tc.address, got)
	}
	if got := resolver.Resolve(netip.MustParseAddr("127.0.0.1")); got != nil {
		t.Fatal("loopback resolved")
	}
	if err := resolver.Close(); err != nil {
		t.Fatal(err)
	}
	if got := resolver.Resolve(netip.MustParseAddr("8.8.8.8")); got != nil {
		t.Fatal("closed resolver returned data")
	}
}
