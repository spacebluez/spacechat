package kaomoji

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Item is a single kaomoji entry. Keywords is reserved for future search and
// may be empty in the first release.
type Item struct {
	Text     string
	Keywords []string
}

// Category is a named group of kaomoji entries.
type Category struct {
	ID    string
	Name  string
	Items []Item
}

// Catalog is the built-in kaomoji library. Keep every entry single-line,
// valid UTF-8 and free of control characters so it always passes the client
// message validation used for regular chat text.
var Catalog = []Category{
	{ID: "happy", Name: "开心", Items: []Item{
		{Text: "(◕‿◕✿)"},
		{Text: "(*^▽^*)"},
		{Text: "ヾ(≧▽≦*)o"},
		{Text: "ヽ(✿ﾟ▽ﾟ)ノ"},
		{Text: "(≧∇≦)ﾉ"},
		{Text: "╰(*°▽°*)╯"},
	}},
	{ID: "sad", Name: "难过", Items: []Item{
		{Text: "(╥﹏╥)"},
		{Text: "(｡•́︿•̀｡)"},
		{Text: "(T_T)"},
		{Text: "(ノへ￣、)"},
		{Text: "(´;ω;｀)"},
		{Text: "(ノ_<。)"},
	}},
	{ID: "surprise", Name: "惊讶", Items: []Item{
		{Text: "(⊙_⊙)"},
		{Text: "Σ(っ °Д °;)っ"},
		{Text: "(°ロ°)"},
		{Text: "⊙０⊙"},
		{Text: "(⊙ˍ⊙)"},
		{Text: "(°Д°)"},
	}},
	{ID: "angry", Name: "生气", Items: []Item{
		{Text: "(╬ Ò﹏Ó)"},
		{Text: "(￣^￣)ゞ"},
		{Text: "(｀皿´)"},
		{Text: "(¬_¬ )"},
		{Text: "(ノಠ益ಠ)ノ"},
		{Text: "(ㆆ_ㆆ)"},
	}},
	{ID: "cute", Name: "卖萌", Items: []Item{
		{Text: "(๑•̀ㅂ•́)و✧"},
		{Text: "(๑¯◡¯๑)"},
		{Text: "(｡•ᴗ•｡)"},
		{Text: "(´• ω •)"},
		{Text: "(・ω・)"},
		{Text: "(￣▽￣)~*"},
	}},
	{ID: "action", Name: "动作", Items: []Item{
		{Text: "(ง •_•)ง"},
		{Text: "(ノ≧∀≦)ノ"},
		{Text: "_(:з」∠)_"},
		{Text: "(づ｡◕‿‿◕｡)づ"},
		{Text: "(๑•̀ㅂ•́)و"},
		{Text: "ヽ(ﾟ∀ﾟ)ﾉ"},
	}},
	{ID: "greeting", Name: "问候", Items: []Item{
		{Text: "(｡･∀･)ﾉﾞ"},
		{Text: "(＾▽＾)/"},
		{Text: "(´▽｀)ノ♪"},
		{Text: "(￣▽￣)ノ"},
		{Text: "(´･ω･)ﾉ"},
		{Text: "ヾ(＾∇＾)"},
	}},
	{ID: "animal", Name: "动物", Items: []Item{
		{Text: "(=^･ω･^=)"},
		{Text: "(￣(工)￣)"},
		{Text: "ʕ •ᴥ•ʔ"},
		{Text: "(^・ω・^ )"},
		{Text: "(◕ᴥ◕)"},
		{Text: "(=^-ω-^=)"},
	}},
	{ID: "other", Name: "搞怪", Items: []Item{
		{Text: "(¯\\_(ツ)_/¯)"},
		{Text: "(╯°□°)╯︵ ┻━┻"},
		{Text: "┬─┬ノ( º _ ºノ)"},
		{Text: "( ͡° ͜ʖ ͡°)"},
		{Text: "(⌐■_■)"},
		{Text: "(￣ー￣)"},
	}},
}

// Categories returns a defensive copy of the built-in catalog so callers can
// reorder or trim their local view without mutating the global library.
func Categories() []Category {
	categories := make([]Category, len(Catalog))
	for i, category := range Catalog {
		categories[i] = Category{ID: category.ID, Name: category.Name, Items: append([]Item(nil), category.Items...)}
	}
	return categories
}

// ValidateCatalog verifies the catalog invariants required by the picker and
// by protocol.ValidateBody.
func ValidateCatalog() error {
	if len(Catalog) == 0 {
		return fmt.Errorf("catalog must not be empty")
	}
	ids := make(map[string]bool, len(Catalog))
	for _, category := range Catalog {
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
