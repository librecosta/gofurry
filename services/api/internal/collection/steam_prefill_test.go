package collection

import (
	"testing"
	"uuid"
)

func TestSteamPrefillObservationProvenance(t *testing.T) {
	const provenance = `"app_id":42,"tier":"observed_storefront","source_url":"https://store.steampowered.com/api/appdetails"`
	id := uuid.NewV7()
	observed := observation{identity: identity{Capability: intakeCapability}, ID: id, State: "present",
		Scope: segmentScope{scope: scope{Locale: "zh-cn", Region: "cn"}, Dimensions: map[string]string{"section": "details"}}}
	observed.Data = []byte(`{` + provenance + `,"name":"示例游戏","description":"森林里的探险。","type":"game","developers":["Studio"]}`)
	zh := PrefillScope{Locale: "zh-cn"}
	applyPrefillObservation(&zh, "42", observed)
	if zh.Name != "示例游戏" || zh.Observation == nil || *zh.Observation != id || zh.FactsObservation == nil || *zh.FactsObservation != id {
		t.Fatalf("missing Chinese provenance: %+v", zh)
	}
	observed.ID = uuid.NewV7()
	observed.Data = []byte(`{` + provenance + `,"name":"English only","description":"English fallback","type":"game","publishers":["Example"]}`)
	applyPrefillObservation(&zh, "42", observed)
	blank := PrefillScope{Locale: "zh-cn"}
	applyPrefillObservation(&blank, "42", observed)
	if zh.Name != "示例游戏" || *zh.Observation != id || blank.Name != "" || blank.Observation != nil ||
		blank.FactsObservation == nil || *blank.FactsObservation != observed.ID || len(blank.Publishers) != 1 {
		t.Fatalf("English fallback altered Chinese attribution: stored=%+v new=%+v", zh, blank)
	}
	observed.Scope.Dimensions["section"] = "media"
	observed.ID = uuid.NewV7()
	observed.Data = []byte(`{` + provenance + `,"header":"https://cdn.steamstatic.com/title.jpg","screenshots":["http://not-permitted.example/example.jpg"]}`)
	applyPrefillObservation(&blank, "42", observed)
	if blank.Name != "" || blank.MediaObservation == nil || *blank.MediaObservation != observed.ID ||
		blank.Media.Header != "https://cdn.steamstatic.com/title.jpg" || len(blank.Media.Screenshots) != 0 {
		t.Fatalf("partial result should retain only validated media: %+v", blank)
	}
}
