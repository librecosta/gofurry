package legacyimport

import (
	"strings"
	"testing"

	"github.com/librecosta/gofurry/services/api/internal/catalog"
)

func TestInitializationInventory(t *testing.T) {
	site := makeKey(sites, 1)
	category := makeKey(categories, 1)
	tag := makeKey(tags, 1)
	name := []catalog.Localization{{Locale: "en", Name: "Fixture"}}
	for _, tc := range []struct {
		name string
		edit func(*blueprint)
		want string
	}{
		{name: "draft unclassified Site without address or dedicated Logo"},
		{name: "invalid source reference", edit: func(b *blueprint) { b.set(site, "deferred") }, want: "unresolved references"},
		{name: "missing name", edit: func(b *blueprint) { b.Items[0].Editorial.Localizations = nil }, want: "editorial name"},
		{name: "unresolved identity", edit: func(b *blueprint) { b.Items[0].IdentityIssue = "slug_required" }, want: "unresolved identity"},
		{name: "unresolved association", edit: func(b *blueprint) { b.Items[0].RelationIssue = "relationship_invalid" }, want: "unresolved identity"},
		{name: "unmapped category", edit: func(b *blueprint) { b.Items[2].Category = makeKey(categories, 2) }, want: "mapped category"},
		{name: "unverified supplied Logo", edit: func(b *blueprint) { b.Items[0].LogoIssue = "logo_approval_pending" }, want: "declared Site Logo"},
		{name: "unreadable source Logo", edit: func(b *blueprint) { b.Items[0].LogoIssue = "logo_unavailable_in_seed" }, want: "declared Site Logo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := blueprint{Rows: map[string]sourceResult{site.String(): {Source: site}},
				Items: []intent{
					{Key: site, Editorial: catalog.Editorial{Slug: "fixture", Localizations: name}},
					{Key: category, Editorial: catalog.Editorial{Localizations: name}},
					{Key: tag, Category: category, Editorial: catalog.Editorial{Localizations: name}},
				},
			}
			if tc.edit != nil {
				tc.edit(&b)
			}
			inventory, err := bootstrapInventory(b, nil, BootstrapInitialization)
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("got %v, want %q", err, tc.want)
				}
				return
			}
			if err != nil || inventory.Projection["sites_using_default_logo"] != 1 {
				t.Fatalf("default Logo rejected: %v, %v", inventory.Projection, err)
			}
		})
	}
	if _, err := bootstrapInventory(blueprint{}, []decision{{Operation: "allow_publish"}}, BootstrapInitialization); err == nil {
		t.Fatal("initialization accepted a publication decision")
	}
}

func TestInitializationPlanScope(t *testing.T) {
	var target Config
	target.Database.Host, target.Database.Port = "127.0.0.1", 5432
	legacy := target.target()
	target.bootstrapPackage = strings.Repeat("a", 64)
	initialization := target.target()
	target.bootstrapPackage = strings.Repeat("b", 64)
	otherArchive := target.target()
	target.Database.Port = 5433
	otherEndpoint := target.target()
	if legacy == initialization || initialization == otherArchive || otherArchive == otherEndpoint {
		t.Fatal("bootstrap v2 plan scope did not distinguish archive or endpoint")
	}
}
