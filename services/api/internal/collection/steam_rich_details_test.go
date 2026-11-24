package collection

import (
	"encoding/json/jsontext"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var richSampleDir = flag.String("steam-rich-samples", "", "Private v3 normalized Harvest payloads")

func TestNormalizeSteamRichDetails(t *testing.T) {
	o := observation{identity: identity{Capability: intakeCapability, Schema: 3},
		State: "present", ExecutionEgressRegion: "cn",
		Scope: segmentScope{scope: scope{Locale: "en", Region: "hk", EgressRegion: "cn"}, Dimensions: map[string]string{"section": "rich_details"}},
		Data:  jsontext.Value(`{"app_id":434000,"tier":"observed_storefront","source_url":"https://store.steampowered.com/api/appdetails","detailed_description":"<h2>Heading</h2>","required_age":0}`)}
	c, err := normalizeSteam(o, "434000")
	if err != nil || !sameParams(c.Data, o.Data) {
		t.Fatalf("v3 rich details: %v", err)
	}
	if _, err = normalizeSteam(o, "1332010"); !errors.Is(err, errMediaIdentity) {
		t.Fatalf("app identity: %v", err)
	}
	o.Schema = 2
	if _, err = normalizeSteam(o, "434000"); err == nil {
		t.Fatal("v2 accepted the v3 segment")
	}
	if *richSampleDir == "" {
		return
	}
	o.Schema = 3
	for _, app := range []string{"434000", "1332010", "1145360"} {
		t.Run(app, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(*richSampleDir, app+"-rich-v3.json"))
			if err != nil {
				t.Fatal(err)
			}
			o.Data = data
			if _, err = normalizeSteam(o, app); err != nil {
				t.Fatalf("private sample normalization: %v", err)
			}
		})
	}
}
