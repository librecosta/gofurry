package catalog

import (
	"net/url"
	"strings"
	"testing"
)

func TestSiteIntakeSlug(t *testing.T) {
	for _, test := range []struct{ address, slug string }{
		{"https://www2.Art--Archive.Example.org:9443/gallery", "art-archive-example-org"},
		{"https://www.例子.example/", "xn-fsqu00a-example"},
		{"https://www.example.org", "example-org"},
		{"http://[2001:db8::1]/", "2001-db8-1"},
		{"https://" + strings.Repeat("a", 60) + "." + strings.Repeat("b", 60) + ".example", strings.Repeat("a", 60) + "-" + strings.Repeat("b", 19)},
	} {
		canonical, err := normalizeURL(test.address)
		if err != nil {
			t.Fatal(err)
		}
		u, err := url.Parse(canonical)
		if err != nil {
			t.Fatal(err)
		}
		if slug := siteIntakeSlug(u); slug != test.slug || len(slug) > 80 || !slugPattern.MatchString(slug) {
			t.Errorf("slug for %q = %q, want %q", test.address, slug, test.slug)
		}
	}
}
