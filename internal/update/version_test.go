package update

import "testing"

func TestParseVersionAndDecision(t *testing.T) {
	valid := map[string]string{
		"0.0.0":    "0.0.0",
		"0.4.0":    "0.4.0",
		"12.34.56": "12.34.56",
	}
	for input, expected := range valid {
		version, err := ParseVersion(input)
		if err != nil || version.String() != expected {
			t.Fatalf("ParseVersion(%q) = %v, %v", input, version, err)
		}
	}
	for _, input := range []string{"", "v1.2.3", "01.2.3", "1.02.3", "1.2", "1.2.3-beta", "1.2.3.4", "-1.2.3"} {
		if _, err := ParseVersion(input); err == nil {
			t.Fatalf("ParseVersion accepted %q", input)
		}
	}

	minimum, _ := ParseVersion("0.3.2")
	latest, _ := ParseVersion("0.4.0")
	cases := map[string]Decision{
		"0.3.1": DecisionRequired,
		"0.3.2": DecisionOptional,
		"0.4.0": DecisionCurrent,
		"0.5.0": DecisionAhead,
	}
	for text, expected := range cases {
		current, _ := ParseVersion(text)
		if actual := DecisionFor(current, latest, minimum); actual != expected {
			t.Fatalf("DecisionFor(%s) = %v, want %v", text, actual, expected)
		}
	}
}

func TestParseVersionRejectsOverflow(t *testing.T) {
	if _, err := ParseVersion("18446744073709551616.0.0"); err == nil {
		t.Fatal("accepted component larger than uint64")
	}
}
