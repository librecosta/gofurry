package collection

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"slices"
	"strconv"

	"github.com/gofurry/steam-go/web/storefront"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
)

func richSteamDetails(o observation) bool {
	return o.Capability == intakeCapability && o.Schema == 3 && o.Scope.Dimensions["section"] == "rich_details"
}

func normalizeSteamRichDetails(o observation, app string) (compact, error) {
	c := compact{Facts: map[string]jsontext.Value{}, Ruleset: "gofurry-steam-rich-3"}
	var v catalog.SteamRichSource
	if !richSteamDetails(o) || !steamScopeOK(o) || !scopePart.MatchString(o.ExecutionEgressRegion) || o.State != "present" ||
		len(o.Data) > catalog.SteamRichDataLimit || json.Unmarshal(o.Data, &v, json.RejectUnknownMembers(true)) != nil ||
		!validRichSource(v) {
		return c, errProtocol
	}
	if strconv.FormatUint(uint64(v.AppID), 10) != app {
		return c, errMediaIdentity
	}
	var languages []storefront.LanguageSupport
	if v.SupportedLanguages != nil {
		languages = storefront.ParseSupportedLanguages(*v.SupportedLanguages)
	}
	if !slices.EqualFunc(languages, v.Languages, func(a, b storefront.LanguageSupport) bool {
		l, _ := json.Marshal(a)
		r, _ := json.Marshal(b)
		return string(l) == string(r)
	}) {
		return c, errProtocol
	}
	raw, err := json.Marshal(v, json.Deterministic(true))
	if err != nil || len(raw) > catalog.SteamRichDataLimit || !sameParams(raw, o.Data) {
		return c, errProtocol
	}
	c.Data = raw
	return c, nil
}

func validRichSource(v catalog.SteamRichSource) bool {
	return v.AppID != 0 && v.Tier == "observed_storefront" && v.URL == "https://store.steampowered.com/api/appdetails" &&
		(v.RequiredAge == nil || *v.RequiredAge >= 0 && *v.RequiredAge <= 150) && v.ValidDiagnostics()
}
