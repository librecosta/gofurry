package catalog

import (
	"errors"
	"slices"
	"testing"
	"time"
	"uuid"
)

func TestEditPreviouslySavedGameMedia(t *testing.T) {
	source, observation, latest := uuid.NewV7(), uuid.NewV7(), uuid.NewV7()
	at := time.Now().UTC()
	const a, b, c = "a", "b", "c"
	old := []GameDraftMedia{{GameMediaSelection: GameMediaSelection{
		Locale: "en", Media: SteamMedia{Screenshots: []string{a, b, c}}, CoverURL: a,
		MediaSourceID: &source, MediaObservationID: &observation,
	}, MediaStoreCountry: "us", MediaObservedAt: &at}}
	candidates := []GameSourceCandidate{{
		Localization: Localization{Locale: "en"}, Section: "media", SourceID: source,
		ObservationID: latest, Media: SteamMedia{Screenshots: []string{"latest"}},
	}}
	choice := old[0].GameMediaSelection
	choice.CoverURL = ""
	choice.Media.Screenshots = []string{c, b}
	got, err := gameMediaSelections([]GameMediaSelection{choice}, old, candidates)
	if err != nil || len(got) != 1 || got[0].CoverURL != "" ||
		!slices.Equal(got[0].Media.Screenshots, []string{c, b}) ||
		got[0].MediaStoreCountry != "us" || got[0].MediaObservedAt == nil || !got[0].MediaObservedAt.Equal(at) {
		t.Fatalf("saved verified selection was not retained: %+v, %v", got, err)
	}
	for _, test := range []struct {
		items []string
		batch *uuid.UUID
	}{
		{[]string{c, "latest"}, &observation},
		{[]string{c}, &latest},
		{[]string{b, b}, &observation},
		{[]string{b}, nil},
	} {
		choice.Media.Screenshots = test.items
		choice.MediaObservationID = test.batch
		if _, err = gameMediaSelections([]GameMediaSelection{choice}, old, candidates); !errors.Is(err, ErrInvalid) {
			t.Fatalf("unverified selection passed: %+v, %v", test, err)
		}
	}
}
