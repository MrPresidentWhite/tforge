package main

import (
	"strings"
	"testing"
)

func TestParseEnvFile(t *testing.T) {
	input := strings.Join([]string{
		"# a comment",
		"",
		"PLAIN=value",
		"  SPACED  =  spaced value  ",
		`QUOTED="quoted value"`,
		"SINGLE='single quoted'",
		"export EXPORTED=from-export",
		"EMPTY=",
		"NO_EQUALS_SIGN",
		"WITH_EQUALS=a=b=c",
		"KEEP_INNER_SPACE=\"  padded  \"",
		"DUPLICATE=first",
		"DUPLICATE=second",
	}, "\n")

	pairs, err := parseEnvFile(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseEnvFile: %v", err)
	}

	got := make(map[string]string, len(pairs))
	for _, p := range pairs {
		got[p.Key] = p.Value
	}

	want := map[string]string{
		"PLAIN":            "value",
		"SPACED":           "spaced value",
		"QUOTED":           "quoted value",
		"SINGLE":           "single quoted",
		"EXPORTED":         "from-export",
		"EMPTY":            "",
		"WITH_EQUALS":      "a=b=c",
		"KEEP_INNER_SPACE": "  padded  ",
		"DUPLICATE":        "second",
	}

	if len(got) != len(want) {
		t.Fatalf("parsed %d keys, want %d: %v", len(got), len(want), got)
	}
	for k, wantVal := range want {
		if gotVal, ok := got[k]; !ok {
			t.Errorf("missing key %q", k)
		} else if gotVal != wantVal {
			t.Errorf("%s = %q, want %q", k, gotVal, wantVal)
		}
	}

	if _, ok := got["NO_EQUALS_SIGN"]; ok {
		t.Error("a line without '=' should be skipped")
	}
}

func TestParseEnvFilePreservesOrderAndDeduplicates(t *testing.T) {
	pairs, err := parseEnvFile(strings.NewReader("B=1\nA=2\nB=3\n"))
	if err != nil {
		t.Fatalf("parseEnvFile: %v", err)
	}

	if len(pairs) != 2 {
		t.Fatalf("got %d pairs, want 2: %+v", len(pairs), pairs)
	}
	if pairs[0].Key != "B" || pairs[0].Value != "3" {
		t.Errorf("first pair = %+v, want B=3 (last assignment wins, first position kept)", pairs[0])
	}
	if pairs[1].Key != "A" || pairs[1].Value != "2" {
		t.Errorf("second pair = %+v, want A=2", pairs[1])
	}
}

func TestUnquoteEnvValue(t *testing.T) {
	cases := []struct{ in, want string }{
		{`plain`, "plain"},
		{`  padded  `, "padded"},
		{`"double"`, "double"},
		{`'single'`, "single"},
		{`"  inner  "`, "  inner  "},
		{`"unbalanced`, `"unbalanced`},
		{`unbalanced"`, `unbalanced"`},
		{`"`, `"`},
		{``, ``},
		{`"a" and "b"`, `a" and "b`},
	}

	for _, c := range cases {
		if got := unquoteEnvValue(c.in); got != c.want {
			t.Errorf("unquoteEnvValue(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

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
