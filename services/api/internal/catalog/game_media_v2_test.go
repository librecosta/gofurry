package catalog

import (
	"errors"
	"strconv"
	"testing"
	"uuid"
)

func TestGameMediaEditorialLimits(t *testing.T) {
	origin := GameMediaOrigin{GameSourceID: uuid.NewV7(), ObservationID: uuid.NewV7()}
	c := GameMediaCandidateV2{GameMediaOrigin: origin, Media: &SteamAppMediaV2{}}
	edit := GameMediaLocaleEdit{Locale: "en"}
	pick := func(id string) GameMediaPick {
		return GameMediaPick{GameSourceID: origin.GameSourceID, ObservationID: origin.ObservationID, ID: id}
	}
	for i := range 9 {
		id := strconv.Itoa(i)
		c.Media.Screenshots = append(c.Media.Screenshots, SteamScreenshot{ID: id, Order: i, Full: "https://shared.steamstatic.com/image.png"})
		edit.Screenshots = append(edit.Screenshots, pick(id))
	}
	for i := range 5 {
		id := strconv.Itoa(10 + i)
		c.Media.Movies = append(c.Media.Movies, SteamMovie{ID: id, Order: i, Sources: SteamMovieSources{DASHAV1: "https://video.steamstatic.com/source.mpd"}})
		edit.Movies = append(edit.Movies, pick(id))
	}
	lookup := func(GameMediaPick) (GameMediaCandidateV2, error) { return c, nil }
	if _, err := resolveGameMedia(edit, GameMediaSelectionV2{}, lookup); !errors.Is(err, ErrInvalid) {
		t.Fatal("accepted nine screenshots and five videos")
	}
	edit.Movies = edit.Movies[:4]
	if _, err := resolveGameMedia(edit, GameMediaSelectionV2{}, lookup); !errors.Is(err, ErrInvalid) {
		t.Fatal("accepted nine screenshots")
	}
	edit.Screenshots = edit.Screenshots[:8]
	saved, err := resolveGameMedia(edit, GameMediaSelectionV2{}, lookup)
	if err != nil || len(saved.Movies) != 4 || len(saved.Screenshots) != 8 {
		t.Fatal("8/4 choice missing")
	}
	if len(saved.legacyMedia(origin.GameSourceID).Movies) != 0 {
		t.Fatal("manifest was published as a direct file")
	}
	edit.Screenshots[1] = edit.Screenshots[0]
	if _, err := resolveGameMedia(edit, saved, lookup); !errors.Is(err, ErrInvalid) {
		t.Fatal("accepted duplicate stable ID")
	}
}

func TestGameMediaImagesPreserveRoles(t *testing.T) {
	origin := GameMediaOrigin{GameSourceID: uuid.NewV7(), ObservationID: uuid.NewV7()}
	url := "https://shared.steamstatic.com/store_item_assets/steam/apps/434000/image.png"
	c := GameMediaCandidateV2{GameMediaOrigin: origin, Assets: &SteamAssetsV1{Assets: []SteamAsset{
		{ID: "capsule", Type: "library_capsule", Family: "library", Slot: "library_capsule", URL: url},
		{ID: "hero", Type: "library_hero", Family: "library", Slot: "library_hero", URL: url},
		{ID: "logo", Type: "library_logo", Family: "library", Slot: "logo", URL: url},
	}}}
	pick := func(id string) GameMediaPick {
		return GameMediaPick{GameSourceID: origin.GameSourceID, ObservationID: origin.ObservationID, ID: id}
	}
	edit := GameMediaLocaleEdit{Locale: "en", Images: []GameMediaPick{pick("capsule"), pick("hero"), pick("logo")}}
	lookup := func(GameMediaPick) (GameMediaCandidateV2, error) { return c, nil }
	next, err := resolveGameMedia(edit, GameMediaSelectionV2{}, lookup)
	if err != nil || len(next.Images) != 3 || next.Images[0].Type == next.Images[1].Type || next.Images[2].Slot != "logo" {
		t.Fatal("distinct roles sharing URL were collapsed")
	}
	edit.Images = edit.Images[:2]
	logo := pick("logo")
	edit.Cover = &logo
	if _, err := resolveGameMedia(edit, next, lookup); !errors.Is(err, ErrInvalid) {
		t.Fatal("accepted logo as primary cover")
	}
}
