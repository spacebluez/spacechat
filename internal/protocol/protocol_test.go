package protocol

import (
	"strings"
	"testing"
)

func TestValidation(t *testing.T) {
	for _, name := range []string{"小明", "Alice", strings.Repeat("中", 20)} {
		if err := ValidateName(name); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"", "  ", " alice", "a\nb", "a\x1bb", strings.Repeat("中", 21)} {
		if ValidateName(name) == nil {
			t.Fatalf("accepted name %q", name)
		}
	}
	for _, body := range []string{"你好世界", strings.Repeat("中", 2000)} {
		if err := ValidateBody(body); err != nil {
			t.Fatal(err)
		}
	}
	for _, body := range []string{"", "   ", "a\nb", "\x1b[31m", strings.Repeat("中", 2001)} {
		if ValidateBody(body) == nil {
			t.Fatalf("accepted body %q", body)
		}
	}
}
