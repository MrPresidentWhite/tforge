package main

import (
	"strings"
	"testing"
)

func TestFormatExportSortsKeys(t *testing.T) {
	env := map[string]string{"ZED": "1", "ALPHA": "2", "MID": "3"}

	out, err := formatExport(env, "raw")
	if err != nil {
		t.Fatalf("formatExport: %v", err)
	}

	want := "ALPHA=2\nMID=3\nZED=1\n"
	if out != want {
		t.Errorf("formatExport = %q, want %q", out, want)
	}
}

func TestFormatExportShellQuotesDangerousValues(t *testing.T) {
	env := map[string]string{
		"SPACES":   "two words",
		"SEMI":     "value; rm -rf /",
		"QUOTE":    "it's here",
		"SUBSHELL": "$(whoami)",
	}

	out, err := formatExport(env, "shell")
	if err != nil {
		t.Fatalf("formatExport: %v", err)
	}

	want := strings.Join([]string{
		`QUOTE='it'\''s here'`,
		`SEMI='value; rm -rf /'`,
		`SPACES='two words'`,
		`SUBSHELL='$(whoami)'`,
		"",
	}, "\n")

	if out != want {
		t.Errorf("formatExport(shell) =\n%q\nwant\n%q", out, want)
	}
}

func TestFormatExportRejectsUnknownFormat(t *testing.T) {
	if _, err := formatExport(map[string]string{"A": "1"}, "yaml"); err == nil {
		t.Error("expected an error for an unsupported export format")
	}
}

func TestShellQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"simple", `'simple'`},
		{"", `''`},
		{"with space", `'with space'`},
		{"it's", `'it'\''s'`},
		{"a'b'c", `'a'\''b'\''c'`},
		{"line\nbreak", "'line\nbreak'"},
	}

	for _, c := range cases {
		if got := shellQuote(c.in); got != c.want {
			t.Errorf("shellQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
