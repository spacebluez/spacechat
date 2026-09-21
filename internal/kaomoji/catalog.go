package kaomoji

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"xchat/internal/protocol"
)

type Item struct {
	Text     string   `json:"text"`
	Keywords []string `json:"keywords,omitempty"`
}

type Category struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Items []Item `json:"items"`
}

const MaxCatalogBytes = 1024 * 1024

type Catalog struct {
	Version    int        `json:"version"`
	Categories []Category `json:"categories"`
}

//go:embed defaults.json
var defaults []byte

func Default() Catalog {
	catalog, err := Parse(defaults)
	if err != nil {
		panic(err)
	}
	return catalog
}

func Parse(data []byte) (Catalog, error) {
	var next Catalog
	if len(data) > MaxCatalogBytes {
		return Catalog{}, fmt.Errorf("kaomoji catalog exceeds %d bytes", MaxCatalogBytes)
	}
	if err := json.Unmarshal(data, &next); err != nil {
		return Catalog{}, fmt.Errorf("parse kaomoji catalog: %w", err)
	}
	if next.Version != 1 {
		return Catalog{}, fmt.Errorf("unsupported kaomoji catalog version %d", next.Version)
	}
	if err := validate(next.Categories); err != nil {
		return Catalog{}, err
	}
	return next, nil
}

func ReadFile(path string) (Catalog, error) {
	file, err := os.Open(path)
	if err != nil {
		return Catalog{}, fmt.Errorf("read kaomoji catalog: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaxCatalogBytes+1))
	if err != nil {
		return Catalog{}, fmt.Errorf("read kaomoji catalog: %w", err)
	}
	return Parse(data)
}

func (catalog Catalog) Clone() Catalog {
	groups := slices.Clone(catalog.Categories)
	for i := range groups {
		groups[i].Items = slices.Clone(groups[i].Items)
		for j := range groups[i].Items {
			groups[i].Items[j].Keywords = slices.Clone(groups[i].Items[j].Keywords)
		}
	}
	return Catalog{Version: catalog.Version, Categories: groups}
}

func ValidateItem(text string) error {
	if strings.ContainsAny(text, "\r\n") {
		return fmt.Errorf("kaomoji must be single-line")
	}
	return protocol.ValidateBody(text)
}

func validate(groups []Category) error {
	if len(groups) == 0 || len(groups) > 64 {
		return fmt.Errorf("kaomoji catalog must contain 1 to 64 categories")
	}
	ids := make(map[string]bool)
	count := 0
	for _, group := range groups {
		if ValidateItem(group.ID) != nil || ids[group.ID] {
			return fmt.Errorf("invalid or duplicate category ID %q", group.ID)
		}
		ids[group.ID] = true
		if err := ValidateItem(group.Name); err != nil {
			return fmt.Errorf("category name: %w", err)
		}
		if len(group.Items) == 0 {
			return fmt.Errorf("category %q has no items", group.ID)
		}
		count += len(group.Items)
		if count > 4096 {
			return fmt.Errorf("kaomoji catalog exceeds 4096 items")
		}
		seen := make(map[string]bool)
		for _, item := range group.Items {
			if err := ValidateItem(item.Text); err != nil {
				return fmt.Errorf("category %q: %w", group.ID, err)
			}
			if seen[item.Text] {
				return fmt.Errorf("duplicate kaomoji in category %q", group.ID)
			}
			seen[item.Text] = true
			if len(item.Keywords) > 32 {
				return fmt.Errorf("kaomoji has more than 32 keywords")
			}
			for _, keyword := range item.Keywords {
				if err := ValidateItem(keyword); err != nil {
					return fmt.Errorf("keyword: %w", err)
				}
			}
		}
	}
	return nil
}
