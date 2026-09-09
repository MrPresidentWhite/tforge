package envfile

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
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
		`KEEP_INNER_SPACE="  padded  "`,
		"DUPLICATE=first",
		"DUPLICATE=second",
	}, "\n")

	pairs, err := ParseString(input)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
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

	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsed\n%v\nwant\n%v", got, want)
	}
}

func TestParseKeepsFileOrder(t *testing.T) {
	pairs, err := ParseString("ZED=1\nALPHA=2\nMID=3\n")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if got := Keys(pairs); !reflect.DeepEqual(got, []string{"ZED", "ALPHA", "MID"}) {
		t.Errorf("keys = %v, want file order", got)
	}
}

func TestValidateIgnoresOrder(t *testing.T) {
	structure := []string{"A", "B", "C"}

	forward, _ := ParseString("A=1\nB=2\nC=3\n")
	reverse, _ := ParseString("C=3\nB=2\nA=1\n")

	if r := Validate(structure, forward); !r.OK() {
		t.Errorf("forward order reported issues: %+v", r)
	}
	if r := Validate(structure, reverse); !r.OK() {
		t.Errorf("reverse order reported issues: %+v", r)
	}
}

func TestValidateReportsMissingAndUnknown(t *testing.T) {
	structure := []string{"POSTGRES_HOST", "POSTGRES_PORT", "API_KEY"}
	pairs, _ := ParseString("POSTGRES_HOST=db\nSTRAY_KEY=value\n")

	got := Validate(structure, pairs)

	if !reflect.DeepEqual(got.Missing, []string{"API_KEY", "POSTGRES_PORT"}) {
		t.Errorf("missing = %v, want API_KEY and POSTGRES_PORT sorted", got.Missing)
	}
	if !reflect.DeepEqual(got.Unknown, []string{"STRAY_KEY"}) {
		t.Errorf("unknown = %v, want STRAY_KEY", got.Unknown)
	}
	if got.OK() {
		t.Error("OK() is true despite reported issues")
	}
}

func TestValidateTreatsEmptyValueAsNotSupplied(t *testing.T) {
	structure := []string{"API_KEY"}
	pairs, _ := ParseString("API_KEY=\n")

	got := Validate(structure, pairs)

	if !reflect.DeepEqual(got.Missing, []string{"API_KEY"}) {
		t.Errorf("missing = %v; a key listed without a value has not been supplied", got.Missing)
	}
	if len(got.Unknown) != 0 {
		t.Errorf("unknown = %v, want none", got.Unknown)
	}
}

func TestValidateOnEmptyEnvironment(t *testing.T) {
	structure := []string{"A", "B"}

	got := Validate(structure, nil)

	if !reflect.DeepEqual(got.Missing, []string{"A", "B"}) {
		t.Errorf("missing = %v, want everything", got.Missing)
	}
}

func TestValues(t *testing.T) {
	pairs, _ := ParseString("A=1\nB=\nC=3\n")

	got := Values(pairs)

	want := map[string]string{"A": "1", "C": "3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Values = %v, want %v (empty values dropped)", got, want)
	}
}
