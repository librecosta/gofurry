package cities

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func catalogFixture(t *testing.T, rows string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cities.json")
	if err := os.WriteFile(path, []byte(`{"format":1,"source":"GeoNames cities5000","cities":`+rows+`}`), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

const chengdu = `{"city_id":"1815286","name":"Chengdu","names":{"zh":"成都","en":"Chengdu"},"aliases":["Chengtu","成都","Chengdu"],"country_code":"CN","admin":"Sichuan","latitude":30.67,"longitude":104.07,"population":13568357}`

func TestCatalogSearchAndValidation(t *testing.T) {
	rows := []string{chengdu}
	for i := 0; i < 12; i++ {
		rows = append(rows, strings.NewReplacer("1815286", strconv.Itoa(200000+i), "Chengdu", "Springfield", "Chengtu", "Springfield", "成都", "Springfield").Replace(chengdu))
	}
	// Distinct IDs for many same-name results.
	rows = rows[:12]
	catalog, err := Open(catalogFixture(t, "["+strings.Join(rows, ",")+"]"))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"成都", "chengdu", "CHENGTU"} {
		found := catalog.Search(q)
		if len(found) != 1 || found[0].ID != "1815286" {
			t.Fatalf("name/alias query failed: %s", q)
		}
		encoded, _ := json.Marshal(found)
		if strings.Contains(string(encoded), "latitude") || strings.Contains(string(encoded), "aliases") {
			t.Fatal("search exposed private catalog fields")
		}
	}
	if len(catalog.Search("Springfield")) != 10 || len(catalog.Search("")) != 0 || catalog.Resolve("1815286").Latitude != 30.67 || catalog.Resolve("99999") != nil {
		t.Fatal("bounds/lookup failed")
	}
	for _, q := range []string{strings.Repeat("城", 81), "<script>", "abc\n", "\xff", "../../"} {
		if ValidQuery(q) {
			t.Fatal("invalid query accepted")
		}
	}
	for _, id := range []string{"", "0", "01", "-1", "1815286.0", strings.Repeat("1", 13), "1815286&latitude=0"} {
		if ValidID(id) {
			t.Fatal("invalid ID accepted")
		}
	}
	for _, row := range []string{strings.Replace(chengdu, "30.67", "91", 1), strings.Replace(chengdu, `"latitude":30.67,`, "", 1), chengdu + "," + chengdu, strings.Replace(chengdu, "Chengtu", "bad\\nname", 1)} {
		if _, err := Open(catalogFixture(t, "["+row+"]")); err == nil {
			t.Fatal("invalid catalog accepted")
		}
	}
	if got, err := Open(""); got != nil || err != nil {
		t.Fatal("optional catalog cannot be disabled")
	}
	if _, err := Open(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing catalog accepted")
	}
	if len((*Catalog)(nil).Search("成都")) != 0 || (*Catalog)(nil).Resolve("1815286") != nil {
		t.Fatal("nil catalog did not degrade")
	}
}

func TestLocalGeoNamesCatalog(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "data", "cities", "cities.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("optional local GeoNames snapshot is not initialized")
	}
	catalog, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"成都", "Chengdu", "北京", "Beijing", "Paris", "Springfield"} {
		found := catalog.Search(query)
		if len(found) == 0 || len(found) > 10 {
			t.Fatal("local city query failed", query)
		}
		t.Logf("%s: %s / %s / %s / %s (ID %s)", query, found[0].Names.ZH, found[0].Names.EN, found[0].Admin, found[0].CountryCode, found[0].ID)
	}
}
