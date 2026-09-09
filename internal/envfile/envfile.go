// Package envfile parses env-style files and derives vault structure from
// them.
//
// It exists so the CLI import and the GUI import behave identically: the same
// parsing rules, the same grouping, the same validation. Two implementations
// of "what counts as a group" would drift apart immediately.
package envfile

import (
	"bufio"
	"io"
	"sort"
	"strings"
)

// Pair is one KEY=VALUE assignment.
type Pair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Parse reads env-style input (KEY=VALUE, # comments) and returns the pairs in
// file order.
//
// Beyond minimal splitting it handles the three things real .env files almost
// always contain:
//
//   - a leading "export " on the key,
//   - values wrapped in matching single or double quotes,
//   - surrounding whitespace around unquoted values.
//
// When a key appears more than once the last assignment wins, which matches
// how shells and dotenv loaders behave. The key keeps its first position.
func Parse(r io.Reader) ([]Pair, error) {
	var pairs []Pair
	indexByKey := make(map[string]int)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		key = strings.TrimSpace(key)
		key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
		if key == "" {
			continue
		}

		value = Unquote(value)

		if i, ok := indexByKey[key]; ok {
			pairs[i].Value = value
			continue
		}
		indexByKey[key] = len(pairs)
		pairs = append(pairs, Pair{Key: key, Value: value})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return pairs, nil
}

// ParseString is Parse over a string, for callers that already hold the text.
func ParseString(s string) ([]Pair, error) {
	return Parse(strings.NewReader(s))
}

// Unquote trims a value and removes one layer of matching quotes. Whitespace
// inside quotes is significant and is preserved.
func Unquote(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if first == last && (first == '"' || first == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

// Report is the outcome of checking one environment file against a structure.
//
// Both directions matter. A key with a value that the structure does not know
// about would be silently dropped on import, and a key in the structure with
// no value is what the user most likely wants to fill in before importing.
type Report struct {
	// Missing lists structure keys the environment has no value for.
	Missing []string `json:"missing"`
	// Unknown lists keys the environment supplies a value for that are not
	// part of the structure.
	Unknown []string `json:"unknown"`
}

// OK reports whether the environment matches the structure exactly.
func (r Report) OK() bool { return len(r.Missing) == 0 && len(r.Unknown) == 0 }

// Validate compares an environment's pairs against the structure's keys.
//
// Only non-empty values count as supplied: an env file listing a key with no
// value has not provided it, which is the same situation as leaving it out.
// Comparison is by set membership, so the order of the lines in either file is
// irrelevant. Results are sorted, so the same inputs always produce the same
// warning text.
func Validate(structure []string, pairs []Pair) Report {
	inStructure := make(map[string]bool, len(structure))
	for _, k := range structure {
		inStructure[k] = true
	}

	supplied := make(map[string]bool, len(pairs))
	var report Report

	for _, p := range pairs {
		if p.Value == "" {
			continue
		}
		supplied[p.Key] = true
		if !inStructure[p.Key] {
			report.Unknown = append(report.Unknown, p.Key)
		}
	}

	for _, k := range structure {
		if !supplied[k] {
			report.Missing = append(report.Missing, k)
		}
	}

	sort.Strings(report.Missing)
	sort.Strings(report.Unknown)
	return report
}

// Values turns pairs into a lookup, dropping empty values so callers do not
// have to distinguish "absent" from "present but blank".
func Values(pairs []Pair) map[string]string {
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		if p.Value == "" {
			continue
		}
		out[p.Key] = p.Value
	}
	return out
}

// Keys returns the keys of pairs in order.
func Keys(pairs []Pair) []string {
	out := make([]string, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, p.Key)
	}
	return out
}
