package protocol

import (
	"encoding/json"
	"slices"
	"strings"
	"unicode"
)

func mentionWord(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_'
}

// Quoted mentions use JSON so every valid nickname has an unambiguous form.
func MentionText(name string) string {
	if strings.IndexFunc(name, func(r rune) bool { return !mentionWord(r) }) < 0 {
		return "@" + name
	}
	quoted, _ := json.Marshal([]string{name})
	return "@" + string(quoted)
}

func FindMentions(body string, users []string) []string {
	var found []string
	runes := []rune(body)
	for i := 0; i < len(runes); i++ {
		if runes[i] != '@' || (i > 0 && (mentionWord(runes[i-1]) || runes[i-1] == '@')) {
			continue
		}
		rest := string(runes[i+1:])
		name, consumed := "", 0
		if strings.HasPrefix(rest, "[") {
			decoder := json.NewDecoder(strings.NewReader(rest))
			var names []string
			if decoder.Decode(&names) == nil && len(names) == 1 {
				name = names[0]
				consumed = len([]rune(rest[:decoder.InputOffset()]))
			}
		} else {
			for _, r := range runes[i+1:] {
				if !mentionWord(r) {
					break
				}
				consumed++
			}
			name = string(runes[i+1 : i+1+consumed])
		}
		if slices.Contains(users, name) && !slices.Contains(found, name) {
			found = append(found, name)
		}
		i += consumed
	}
	return found
}
