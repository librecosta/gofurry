package catalog

import (
	"slices"
	"testing"
	"time"

	"github.com/gofurry/steam-go/web/storefront"
)

func TestSteamReleaseAndAvailabilityRemainSeparate(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		raw                             string
		soon                            *bool
		state, precision, from, through string
	}{
		{"2027-05-01", &no, "available", "day", "2027-05-01", "2027-05-01"},
		{"May 2027", &yes, "upcoming", "month", "2027-05-01", "2027-05-31"},
		{"Q3 2027", &yes, "upcoming", "quarter", "2027-07-01", "2027-09-30"},
		{"2027", &yes, "upcoming", "year", "2027-01-01", "2027-12-31"},
		{"Coming Soon", &yes, "upcoming", "tba", "", ""},
		{"", &no, "available", "none", "", ""},
		{"we will see", &yes, "upcoming", "unknown", "", ""},
		{"2027-01-01", nil, "unknown", "day", "2027-01-01", "2027-01-01"},
	}
	for _, test := range cases {
		t.Run(test.state+"/"+test.precision, func(t *testing.T) {
			got := steamReleaseFact(&RichRelease{ComingSoon: test.soon, Date: test.raw})
			if got.State != test.state || got.Precision != test.precision || got.WindowStart != test.from || got.WindowEnd != test.through ||
				(got.ExactDate != "") != (got.Precision == "day") {
				t.Fatalf("normalized release %+v", got)
			}
		})
	}
	raw := "English<strong>*</strong>, French, Imaginary Dialect<br><strong>*</strong>languages with full audio support"
	facts, hasAvailable, err := SteamFactValues(SteamRichSource{SupportedLanguages: &raw, SteamRichContent: SteamRichContent{
		Release: &RichRelease{Date: "2027-01-01"}, Platforms: &RichPlatforms{Linux: &no}, Languages: storefront.ParseSupportedLanguages(raw)}})
	if err != nil || hasAvailable || len(facts) != 2 || facts[0].Kind != "linux" || string(facts[0].Value) != "false" || facts[1].Kind != "languages" {
		t.Fatalf("invalid fields %+v %v", facts, err)
	}
	if got := freshness(time.Time{}, time.Hour); got != "not_observed" {
		t.Fatal(got)
	}
}

func TestSteamChangeRules(t *testing.T) {
	d := steamDetectors[4]
	for _, test := range []struct{ before, after, code string }{
		{`{"state":"upcoming","precision":"quarter","window_start":"2027-04-01"}`, `{"state":"upcoming","precision":"quarter","window_start":"2027-07-01"}`, "game_release_plan_changed"},
		{`{"state":"upcoming","precision":"day","window_start":"2027-04-01","raw_text":"Apr 1, 2027"}`, `{"state":"upcoming","precision":"day","window_start":"2027-04-01","raw_text":"2027-04-01"}`, ""},
		{`{"state":"upcoming"}`, `{"state":"available"}`, "game_became_available"},
		{`{"state":"available"}`, `{"state":"upcoming"}`, "game_availability_withdrawn"},
		{`{"state":"available"}`, `{"state":"unknown"}`, ""},
	} {
		got, err := d.factCodes([]byte(test.before), []byte(test.after))
		if err != nil || test.code == "" && len(got) != 0 || test.code != "" && !slices.Equal(got, []string{test.code}) {
			t.Fatalf("%+v: %v %v", test, got, err)
		}
	}
	oldPrice, newPrice := int64(100), int64(75)
	full, discount := int32(0), int32(25)
	a, b := SteamPriceFact{State: "priced", Currency: "USD", Final: &oldPrice, Discount: &full}, SteamPriceFact{State: "priced", Currency: "USD", Final: &newPrice, Discount: &discount}
	d = steamDetectors[5]
	if got := d.priceCodes(a, b); !slices.Equal(got, []string{"game_price_decreased", "game_discount_started"}) {
		t.Fatal(got)
	}
	if got := d.priceCodes(b, a); !slices.Equal(got, []string{"game_price_increased", "game_discount_ended"}) {
		t.Fatal(got)
	}
	b.Currency = "HKD"
	if got := d.priceCodes(a, b); !slices.Equal(got, []string{"game_price_currency_changed"}) {
		t.Fatal(got)
	}
	b = SteamPriceFact{State: "free", Currency: "USD"}
	if got := d.priceCodes(a, b); !slices.Equal(got, []string{"game_price_state_changed"}) {
		t.Fatal(got)
	}
}
