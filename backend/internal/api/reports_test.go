package api

import "testing"

func TestCSVSafeNeutralisesFormulas(t *testing.T) {
	for in, want := range map[string]string{
		"":                       "",
		"Asha Perera":            "Asha Perera",
		"=HYPERLINK(\"x\")":      "'=HYPERLINK(\"x\")",
		"+94771234567":           "'+94771234567",
		"-5":                     "'-5",
		"@SUM(A1:A9)":            "'@SUM(A1:A9)",
		"asha@example.com":       "asha@example.com", // only a leading character matters
		"2026-03-10":             "2026-03-10",
		"\tTabbed":               "'\tTabbed",
		"Research = Development": "Research = Development",
	} {
		if got := csvSafe(in); got != want {
			t.Errorf("csvSafe(%q) = %q, want %q", in, got, want)
		}
	}
}
