package kaomoji

import (
	"testing"
	"unicode/utf8"
)

func TestCatalogValid(t *testing.T) {
	if err := ValidateCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogEntriesAreSingleLineVisibleText(t *testing.T) {
	for _, category := range Catalog {
		for _, item := range category.Items {
			if !utf8.ValidString(item.Text) {
				t.Fatalf("invalid utf8 in %q: %q", category.ID, item.Text)
			}
			if utf8.RuneCountInString(item.Text) > 2000 {
				t.Fatalf("oversized item in %q", category.ID)
			}
			if item.Text == "" {
				t.Fatalf("empty item in %q", category.ID)
			}
		}
	}
}

func TestCategoriesReturnsCopy(t *testing.T) {
	categories := Categories()
	if len(categories) != len(Catalog) {
		t.Fatalf("category count changed: %d != %d", len(categories), len(Catalog))
	}
	categories[0].Name = "mutated"
	categories[0].Items[0].Text = "mutated"
	if Catalog[0].Name == "mutated" || Catalog[0].Items[0].Text == "mutated" {
		t.Fatal("Categories exposed mutable catalog state")
	}
}

func TestValidateItemRejectsControlAndMultiline(t *testing.T) {
	for _, bad := range []string{"", "  ", "a\nb", "a\rb", "a\x1bb"} {
		if err := ValidateItem(bad); err == nil {
			t.Fatalf("accepted invalid item %q", bad)
		}
	}
}
