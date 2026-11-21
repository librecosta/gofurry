package collection

import (
	"bytes"
	"testing"
	"time"

	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func TestIntakePlansAndProbe(t *testing.T) {
	candidate := dbgen.GameIntakeCandidate{ExternalKey: "949480", Namespace: "public", EgressRegion: "cn"}
	probe, attribution, key := intakeRequest(candidate)
	if probe.Scope != (scope{Locale: "en", Region: "us", EgressRegion: "cn"}) ||
		attribution.scope != probe.Scope || attribution.Dimensions["section"] != "details" ||
		!scopeMatches(attribution, key) {
		t.Fatalf("unexpected Intake probe: %v", attribution)
	}
	want := []struct {
		locale, region, params string
	}{
		{"en", "us", `{"language":"english","store_country":"US"}`},
		{"zh-cn", "cn", `{"language":"schinese","store_country":"CN"}`},
	}
	for i, plan := range intakePlans(candidate.EgressRegion) {
		expected := want[i]
		params, _, policy, err := plan.params("app", candidate.ExternalKey)
		if err != nil || plan.Capability != intakeCapability || plan.Locale != expected.locale ||
			plan.Region != expected.region || plan.EgressRegion != "cn" || policy != "state_refresh" ||
			plan.Interval != 24*time.Hour || !plan.Anchor.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) ||
			string(params) != expected.params {
			t.Fatalf("unexpected recurring plan: %+v, params %s, policy %s, error %v", plan, params, policy, err)
		}
		if i == 0 && !bytes.Equal(probe.Params, params) {
			t.Fatal("existing English probe parameters changed")
		}
	}
}
