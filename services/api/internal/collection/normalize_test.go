package collection

import "testing"

func BenchmarkNormalizeWeb(b *testing.B) {
	o := observation{
		Scope: segmentScope{Dimensions: map[string]string{"section": "html"}},
		State: "present",
		Data:  []byte(`{"title":"Example title","language":"en","canonical":"https://www.example.com/catalog?key=discard","ruleset_version":"web_rules_1","technology":[{"id":"ghost","category":"cms","confidence":"high","evidence":["generator_meta","asset_path"]}]}`),
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := normalizeWeb(o, "https://www.example.com"); err != nil {
			b.Fatal(err)
		}
	}
}
