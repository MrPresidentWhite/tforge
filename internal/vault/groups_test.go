package vault

import (
	"reflect"
	"testing"
)

/* --- DetectGroups ------------------------------------------------------ */

func TestDetectGroupsFromTheExampleInTheDocs(t *testing.T) {
	keys := []string{
		"POSTGRES_HOST",
		"POSTGRES_USER",
		"POSTGRES_PASSWORD",
		"POSTGRES_DB",
		"POSTGRES_PORT",
	}

	groups, ungrouped := DetectGroups(keys)

	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1: %+v", len(groups), groups)
	}
	if groups[0].Prefix != "POSTGRES_" {
		t.Errorf("prefix = %q, want POSTGRES_", groups[0].Prefix)
	}
	if len(groups[0].Keys) != 5 {
		t.Errorf("group holds %d keys, want 5", len(groups[0].Keys))
	}
	if len(ungrouped) != 0 {
		t.Errorf("unexpected leftovers: %v", ungrouped)
	}
}

func TestDetectGroupsNeedsAtLeastTwoKeys(t *testing.T) {
	keys := []string{"POSTGRES_HOST", "POSTGRES_PORT", "REDIS_URL", "DEBUG"}

	groups, ungrouped := DetectGroups(keys)

	if len(groups) != 1 || groups[0].Prefix != "POSTGRES_" {
		t.Fatalf("groups = %+v, want only POSTGRES_", groups)
	}
	if len(ungrouped) != 2 {
		t.Errorf("ungrouped = %v, want REDIS_URL and DEBUG", ungrouped)
	}
}

func TestDetectGroupsExtendsSharedPrefix(t *testing.T) {
	groups, _ := DetectGroups([]string{"NEXT_PUBLIC_API_URL", "NEXT_PUBLIC_SITE_URL"})

	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(groups))
	}
	if groups[0].Prefix != "NEXT_PUBLIC_" {
		t.Errorf("prefix = %q, want NEXT_PUBLIC_", groups[0].Prefix)
	}
}

func TestDetectGroupsStopsAtTheSharedPart(t *testing.T) {
	// The third key shares only NEXT_, so the group must not narrow to
	// NEXT_PUBLIC_ and leave it out.
	groups, ungrouped := DetectGroups([]string{
		"NEXT_PUBLIC_API_URL", "NEXT_PUBLIC_SITE_URL", "NEXT_SECRET",
	})

	if len(groups) != 1 || groups[0].Prefix != "NEXT_" {
		t.Fatalf("groups = %+v, want a single NEXT_ group", groups)
	}
	if len(groups[0].Keys) != 3 {
		t.Errorf("group holds %d keys, want all 3", len(groups[0].Keys))
	}
	if len(ungrouped) != 0 {
		t.Errorf("unexpected leftovers: %v", ungrouped)
	}
}

func TestDetectGroupsKeepsSourceOrder(t *testing.T) {
	groups, _ := DetectGroups([]string{"REDIS_HOST", "REDIS_PORT", "APP_NAME", "APP_ENV"})

	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
	if groups[0].Prefix != "REDIS_" || groups[1].Prefix != "APP_" {
		t.Errorf("groups = %q, %q; want REDIS_ then APP_ as in the source",
			groups[0].Prefix, groups[1].Prefix)
	}
}

func TestDetectGroupsIgnoresDegenerateKeys(t *testing.T) {
	groups, ungrouped := DetectGroups([]string{"_LEADING", "TRAILING_", "PLAIN", "NONE"})

	if len(groups) != 0 {
		t.Errorf("groups = %+v, want none", groups)
	}
	if len(ungrouped) != 4 {
		t.Errorf("ungrouped holds %d keys, want all 4: %v", len(ungrouped), ungrouped)
	}
}

/* --- NormalizeGroups --------------------------------------------------- */

func entriesOf(pairs ...[2]string) []Entry {
	out := make([]Entry, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, Entry{Key: p[0], GroupPrefix: p[1]})
	}
	return out
}

func prefixes(entries []Entry) map[string]string {
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		out[e.Key] = e.GroupPrefix
	}
	return out
}

func TestNormalizeDissolvesAGroupThatFellBelowTheMinimum(t *testing.T) {
	// What is left after deleting four of five POSTGRES_ keys.
	entries := entriesOf([2]string{"POSTGRES_HOST", "POSTGRES_"})

	NormalizeGroups(entries)

	if got := entries[0].GroupPrefix; got != "" {
		t.Errorf("prefix = %q, want empty: one key is not a group", got)
	}
}

func TestNormalizeKeepsAGroupThatStillQualifies(t *testing.T) {
	entries := entriesOf(
		[2]string{"POSTGRES_HOST", "POSTGRES_"},
		[2]string{"POSTGRES_PORT", "POSTGRES_"},
	)

	NormalizeGroups(entries)

	if got := prefixes(entries); got["POSTGRES_HOST"] != "POSTGRES_" || got["POSTGRES_PORT"] != "POSTGRES_" {
		t.Errorf("prefixes = %v, want both to stay in POSTGRES_", got)
	}
}

func TestNormalizeFormsAGroupFromLooseKeys(t *testing.T) {
	// The other direction: two standalone keys that share a prefix.
	entries := entriesOf(
		[2]string{"REDIS_HOST", ""},
		[2]string{"REDIS_PORT", ""},
		[2]string{"DEBUG", ""},
	)

	NormalizeGroups(entries)

	got := prefixes(entries)
	if got["REDIS_HOST"] != "REDIS_" || got["REDIS_PORT"] != "REDIS_" {
		t.Errorf("prefixes = %v, want both REDIS keys grouped", got)
	}
	if got["DEBUG"] != "" {
		t.Errorf("DEBUG was given the prefix %q", got["DEBUG"])
	}
}

func TestNormalizeLetsALooseKeyJoinAnExistingGroup(t *testing.T) {
	// A key added later belongs with the group it obviously fits, even though
	// on its own it would not reach the minimum.
	entries := entriesOf(
		[2]string{"POSTGRES_HOST", "POSTGRES_"},
		[2]string{"POSTGRES_PORT", "POSTGRES_"},
		[2]string{"POSTGRES_PASSWORD", ""},
	)

	NormalizeGroups(entries)

	if got := prefixes(entries)["POSTGRES_PASSWORD"]; got != "POSTGRES_" {
		t.Errorf("prefix = %q, want POSTGRES_", got)
	}
}

func TestNormalizePicksTheMostSpecificExistingGroup(t *testing.T) {
	entries := entriesOf(
		[2]string{"NEXT_SECRET", "NEXT_"},
		[2]string{"NEXT_OTHER", "NEXT_"},
		[2]string{"NEXT_PUBLIC_A", "NEXT_PUBLIC_"},
		[2]string{"NEXT_PUBLIC_B", "NEXT_PUBLIC_"},
		[2]string{"NEXT_PUBLIC_C", ""},
	)

	NormalizeGroups(entries)

	if got := prefixes(entries)["NEXT_PUBLIC_C"]; got != "NEXT_PUBLIC_" {
		t.Errorf("prefix = %q, want the more specific NEXT_PUBLIC_", got)
	}
}

func TestNormalizeKeepsADeliberatelyShorterPrefix(t *testing.T) {
	// DetectGroups would derive NEXT_PUBLIC_ here. An existing group that
	// still qualifies keeps what it has, so a prefix the user chose is not
	// silently rewritten under them.
	entries := entriesOf(
		[2]string{"NEXT_PUBLIC_A", "NEXT_"},
		[2]string{"NEXT_PUBLIC_B", "NEXT_"},
	)

	NormalizeGroups(entries)

	if got := prefixes(entries)["NEXT_PUBLIC_A"]; got != "NEXT_" {
		t.Errorf("prefix = %q, want the chosen NEXT_ to survive", got)
	}
}

func TestNormalizeClearsAPrefixThatNoLongerMatches(t *testing.T) {
	// The key was renamed out of its group.
	entries := entriesOf(
		[2]string{"SOMETHING_ELSE", "POSTGRES_"},
		[2]string{"POSTGRES_HOST", "POSTGRES_"},
		[2]string{"POSTGRES_PORT", "POSTGRES_"},
	)

	NormalizeGroups(entries)

	got := prefixes(entries)
	if got["SOMETHING_ELSE"] != "" {
		t.Errorf("stale prefix %q survived a rename", got["SOMETHING_ELSE"])
	}
	if got["POSTGRES_HOST"] != "POSTGRES_" {
		t.Errorf("the remaining members lost their group: %v", got)
	}
}

func TestNormalizeDissolvesThenRegroups(t *testing.T) {
	// A group of one dissolves, and its key then pairs up with a loose key
	// that shares the prefix.
	entries := entriesOf(
		[2]string{"NEXT_PUBLIC_A", "NEXT_PUBLIC_"},
		[2]string{"NEXT_PUBLIC_B", ""},
	)

	NormalizeGroups(entries)

	got := prefixes(entries)
	if got["NEXT_PUBLIC_A"] != "NEXT_PUBLIC_" || got["NEXT_PUBLIC_B"] != "NEXT_PUBLIC_" {
		t.Errorf("prefixes = %v, want both in NEXT_PUBLIC_", got)
	}
}

func TestNormalizeIsIdempotent(t *testing.T) {
	entries := entriesOf(
		[2]string{"POSTGRES_HOST", "POSTGRES_"},
		[2]string{"POSTGRES_PORT", ""},
		[2]string{"LONE_KEY", "LONE_"},
		[2]string{"DEBUG", ""},
	)

	NormalizeGroups(entries)
	first := prefixes(entries)

	NormalizeGroups(entries)
	second := prefixes(entries)

	if !reflect.DeepEqual(first, second) {
		t.Errorf("a second pass changed things:\n%v\n%v", first, second)
	}
}

func TestNormalizeHandlesEmptyInput(t *testing.T) {
	NormalizeGroups(nil)
	NormalizeGroups([]Entry{})
}

/* --- through the service ----------------------------------------------- */

func TestUpdateVaultNormalisesAfterADeletion(t *testing.T) {
	s := NewService()
	v := s.CreateVault("Vault", "")
	v.Entries = []Entry{
		{Key: "POSTGRES_HOST", GroupPrefix: "POSTGRES_"},
		{Key: "POSTGRES_PORT", GroupPrefix: "POSTGRES_"},
	}
	s.UpdateVault(v)

	// Delete one member, exactly as the editor's row delete does.
	v.Entries = []Entry{{Key: "POSTGRES_HOST", GroupPrefix: "POSTGRES_"}}
	s.UpdateVault(v)

	got, _ := s.GetVault(v.ID)
	if got.Entries[0].GroupPrefix != "" {
		t.Errorf("prefix = %q; a group of one should have dissolved",
			got.Entries[0].GroupPrefix)
	}
}

func TestUpdateVaultNormalisesAfterAnAddition(t *testing.T) {
	s := NewService()
	v := s.CreateVault("Vault", "")
	v.Entries = []Entry{{Key: "REDIS_HOST"}}
	s.UpdateVault(v)

	v.Entries = append(v.Entries, Entry{Key: "REDIS_PORT"})
	s.UpdateVault(v)

	got, _ := s.GetVault(v.ID)
	for _, e := range got.Entries {
		if e.GroupPrefix != "REDIS_" {
			t.Errorf("%s has prefix %q, want REDIS_", e.Key, e.GroupPrefix)
		}
	}
}

func TestSetAllNormalisesWithoutTouchingTheInput(t *testing.T) {
	s := NewService()

	input := []*Vault{{
		ID:   "id-1",
		Name: "Vault",
		Entries: []Entry{
			{Key: "LONE_KEY", GroupPrefix: "LONE_"},
			{Key: "REDIS_HOST"},
			{Key: "REDIS_PORT"},
		},
	}}
	s.SetAll(input)

	got, _ := s.GetVault("id-1")
	byKey := prefixes(got.Entries)

	if byKey["LONE_KEY"] != "" {
		t.Errorf("a group of one survived loading: %q", byKey["LONE_KEY"])
	}
	if byKey["REDIS_HOST"] != "REDIS_" || byKey["REDIS_PORT"] != "REDIS_" {
		t.Errorf("loose keys were not grouped on load: %v", byKey)
	}

	// The caller's slice must not have been rewritten underneath it.
	if input[0].Entries[0].GroupPrefix != "LONE_" {
		t.Error("SetAll modified the caller's entries")
	}
}
