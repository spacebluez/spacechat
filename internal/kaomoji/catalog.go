package kaomoji

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Item is a single kaomoji entry. Keywords is reserved for future search and
// may be empty.
type Item struct {
	Text     string   `json:"text"`
	Keywords []string `json:"keywords,omitempty"`
}

// Category is a named group of kaomoji entries.
type Category struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Items []Item `json:"items"`
}

// file is the on-disk catalog format. Version is reserved so future format
// changes can be rejected or migrated explicitly.
type file struct {
	Version    int        `json:"version"`
	Categories []Category `json:"categories"`
}

var (
	mu      sync.RWMutex
	catalog []Category
)

// Load parses a kaomoji catalog document and atomically replaces the active
// catalog after it has passed validation.
func Load(data []byte) error {
	var document file
	if err := json.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("parse kaomoji catalog: %w", err)
	}
	if document.Version != 1 {
		return fmt.Errorf("unsupported kaomoji catalog version %d", document.Version)
	}
	if err := validate(document.Categories); err != nil {
		return err
	}
	mu.Lock()
	catalog = document.Categories
	mu.Unlock()
	return nil
}

// LoadFile reads path and loads it as a kaomoji catalog.
func LoadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read kaomoji catalog %q: %w", path, err)
	}
	return Load(data)
}

// Categories returns a defensive copy of the active catalog so callers can
// reorder or trim their local view without mutating the shared library.
func Categories() []Category {
	mu.RLock()
	defer mu.RUnlock()
	categories := make([]Category, len(catalog))
	for i, category := range catalog {
		categories[i] = Category{ID: category.ID, Name: category.Name, Items: append([]Item(nil), category.Items...)}
	}
	return categories
}

// ValidateCatalog verifies the catalog invariants required by the picker and
// by protocol.ValidateBody. It checks the currently loaded catalog.
func ValidateCatalog() error {
	mu.RLock()
	defer mu.RUnlock()
	return validate(catalog)
}

func validate(categories []Category) error {
	if len(categories) == 0 {
		return fmt.Errorf("catalog must not be empty")
	}
	ids := make(map[string]bool, len(categories))
	for _, category := range categories {
		if strings.TrimSpace(category.ID) == "" {
			return fmt.Errorf("category ID must not be empty")
		}
		if ids[category.ID] {
			return fmt.Errorf("duplicate category ID %q", category.ID)
		}
		ids[category.ID] = true
		if strings.TrimSpace(category.Name) == "" {
			return fmt.Errorf("category %q name must not be empty", category.ID)
		}
		if len(category.Items) == 0 {
			return fmt.Errorf("category %q must contain at least one item", category.ID)
		}
		seen := make(map[string]bool, len(category.Items))
		for _, item := range category.Items {
			if err := ValidateItem(item.Text); err != nil {
				return fmt.Errorf("category %q: %w", category.ID, err)
			}
			if seen[item.Text] {
				return fmt.Errorf("category %q contains duplicate item %q", category.ID, item.Text)
			}
			seen[item.Text] = true
		}
	}
	return nil
}

// ValidateItem checks the same text constraints as protocol.ValidateBody,
// except that an all-whitespace value is additionally rejected because a
// kaomoji must contain visible characters.
func ValidateItem(text string) error {
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("item must contain visible characters")
	}
	if !utf8.ValidString(text) {
		return fmt.Errorf("item must be valid UTF-8")
	}
	if utf8.RuneCountInString(text) > 2000 {
		return fmt.Errorf("item exceeds 2000 characters")
	}
	if strings.ContainsAny(text, "\r\n") {
		return fmt.Errorf("item must be single-line")
	}
	for _, character := range text {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return fmt.Errorf("item must not contain control characters")
		}
	}
	return nil
}
