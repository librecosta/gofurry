package platform

import (
	"encoding/json/jsontext"
	"strings"
	"testing"
)

func TestJSONV2Contract(t *testing.T) {
	type payload struct {
		Text   string            `json:"text"`
		Number int64             `json:"number"`
		Items  []string          `json:"items"`
		Object map[string]string `json:"object"`
		Raw    jsontext.Value    `json:"raw"`
	}
	want := payload{Text: "中文 <script>", Number: 9007199254740993, Raw: jsontext.Value(`{"ok":true}`)}
	data, err := MarshalJSON(want)
	if err != nil || !jsontext.Value(data).IsValid() {
		t.Fatalf("%s: %v", data, err)
	}
	if !strings.Contains(string(data), `"items":null`) || !strings.Contains(string(data), `"object":null`) || strings.Contains(string(data), "<script>") {
		t.Fatalf("wire compatibility: %s", data)
	}
	var got payload
	if err := UnmarshalJSON(data, &got); err != nil || got.Text != want.Text || got.Number != want.Number {
		t.Fatalf("roundtrip: %+v %v", got, err)
	}
	for _, input := range [][]byte{[]byte(`{"text":"a","text":"b"}`), []byte("{\"text\":\"\xff\"}"), []byte(`{} {}`), []byte(`{invalid`)} {
		if err := UnmarshalJSON(input, &got); err == nil {
			t.Fatalf("ambiguous/malformed input accepted: %q", input)
		}
	}
}
