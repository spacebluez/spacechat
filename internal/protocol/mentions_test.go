package protocol

import (
	"reflect"
	"testing"
)

func TestMentionsMatchWholeMembersAndQuotedNames(t *testing.T) {
	users := []string{"Alice", "Alice2", "小明", "Alice Smith", `a\"b]`, "@all"}
	for _, name := range users {
		body := "Hello " + MentionText(name) + "!"
		if got := FindMentions(body, users); !reflect.DeepEqual(got, []string{name}) {
			t.Errorf("mention %q resolved to %v", body, got)
		}
	}
	for _, body := range []string{"mail@Alice", "@Alice3", "@@Alice", "@unknown", `@["Alice","小明"]`} {
		if got := FindMentions(body, users); len(got) > 0 {
			t.Errorf("false mention in %q: %v", body, got)
		}
	}
	if got := FindMentions("@Alice @Alice2 @Alice\n@小明", users); !reflect.DeepEqual(got, []string{"Alice", "Alice2", "小明"}) {
		t.Fatalf("mentions not deduplicated: %v", got)
	}
}
