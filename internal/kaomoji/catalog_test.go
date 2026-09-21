package kaomoji

import (
	"bytes"
	"testing"
	"unicode/utf8"
)

func TestConfigFileLoadsAndValidates(t *testing.T) {
	catalog, err := ReadFile("defaults.json")
	if err != nil {
		t.Fatal(err)
	}
	categories := catalog.Categories
	if len(categories) == 0 {
		t.Fatal("config file produced an empty catalog")
	}
}

func TestParseAndCloneKeepCatalogsIndependent(t *testing.T) {
	document := []byte(`{
		"version": 1,
		"categories": [
			{"id": "happy", "name": "开心", "items": [{"text": "(◕‿◕✿)"}, {"text": "(*^▽^*)"}]}
		]
	}`)
	catalog, err := Parse(document)
	if err != nil {
		t.Fatal(err)
	}
	categories := catalog.Clone().Categories
	if len(categories) != 1 || categories[0].ID != "happy" || len(categories[0].Items) != 2 {
		t.Fatalf("unexpected catalog: %+v", categories)
	}
	categories[0].Name = "mutated"
	categories[0].Items[0].Text = "mutated"
	loaded := catalog.Categories
	if loaded[0].Name == "mutated" || loaded[0].Items[0].Text == "mutated" {
		t.Fatal("Clone exposed mutable catalog state")
	}
}

func TestParseRejectsInvalidDocuments(t *testing.T) {
	cases := map[string]string{
		"bad json":          `{`,
		"wrong version":     `{"version":2,"categories":[{"id":"happy","name":"开心","items":[{"text":"x"}]}]}`,
		"empty categories":  `{"version":1,"categories":[]}`,
		"empty id":          `{"version":1,"categories":[{"id":" ","name":"开心","items":[{"text":"x"}]}]}`,
		"control in id":     `{"version":1,"categories":[{"id":"a\u001b","name":"开心","items":[{"text":"x"}]}]}`,
		"duplicate ids":     `{"version":1,"categories":[{"id":"a","name":"开心","items":[{"text":"x"}]},{"id":"a","name":"难过","items":[{"text":"y"}]}]}`,
		"empty name":        `{"version":1,"categories":[{"id":"a","name":" ","items":[{"text":"x"}]}]}`,
		"empty items":       `{"version":1,"categories":[{"id":"a","name":"开心","items":[]}]}`,
		"duplicate item":    `{"version":1,"categories":[{"id":"a","name":"开心","items":[{"text":"x"},{"text":"x"}]}]}`,
		"multiline item":    `{"version":1,"categories":[{"id":"a","name":"开心","items":[{"text":"a\nb"}]}]}`,
		"control character": `{"version":1,"categories":[{"id":"a","name":"开心","items":[{"text":"a\u001bb"}]}]}`,
	}
	for name, document := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(document)); err == nil {
				t.Fatal("accepted invalid catalog")
			}
		})
	}

	if _, err := Parse(bytes.Repeat([]byte(" "), MaxCatalogBytes+1)); err == nil {
		t.Fatal("accepted oversized catalog")
	}
}

func TestValidateItemRejectsControlAndMultiline(t *testing.T) {
	for _, bad := range []string{"", "  ", "a\nb", "a\rb", "a\x1bb"} {
		if err := ValidateItem(bad); err == nil {
			t.Fatalf("accepted invalid item %q", bad)
		}
	}
}

func TestValidateItemAcceptsVisibleUnicodeText(t *testing.T) {
	for _, good := range []string{"(◕‿◕✿)", "(¯\\_(ツ)_/¯)", "ʕ •ᴥ•ʔ"} {
		if err := ValidateItem(good); err != nil {
			t.Fatalf("rejected valid item %q: %v", good, err)
		}
		if !utf8.ValidString(good) {
			t.Fatalf("invalid utf8 in %q", good)
		}
	}
}
