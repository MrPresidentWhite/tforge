package vault

import (
	"sort"
	"strings"
)

// MinGroupSize is how many keys must share a prefix before it is a group.
// A single key with an underscore in it is just a key, not a group of one.
const MinGroupSize = 2

// Group is a set of keys sharing a common prefix.
type Group struct {
	Prefix string   `json:"prefix"`
	Keys   []string `json:"keys"`
}

// DetectGroups partitions keys into prefix groups and leftovers.
//
// Keys are first bucketed by their leading segment -- everything up to and
// including the first underscore. A bucket with at least MinGroupSize members
// becomes a group, and its prefix is then extended to the longest underscore
// boundary all its members share, so NEXT_PUBLIC_API_URL and
// NEXT_PUBLIC_SITE_URL group under NEXT_PUBLIC_ rather than NEXT_.
//
// Groups are returned in the order their first key appeared, so the result
// follows the layout of the file or vault the keys came from.
func DetectGroups(keys []string) (groups []Group, ungrouped []string) {
	type bucket struct {
		firstIndex int
		keys       []string
	}

	buckets := make(map[string]*bucket)
	var order []string

	for i, key := range keys {
		seg, ok := leadingSegment(key)
		if !ok {
			ungrouped = append(ungrouped, key)
			continue
		}
		b, seen := buckets[seg]
		if !seen {
			b = &bucket{firstIndex: i}
			buckets[seg] = b
			order = append(order, seg)
		}
		b.keys = append(b.keys, key)
	}

	type placed struct {
		group Group
		at    int
	}
	var found []placed

	for _, seg := range order {
		b := buckets[seg]
		if len(b.keys) < MinGroupSize {
			ungrouped = append(ungrouped, b.keys...)
			continue
		}
		found = append(found, placed{
			group: Group{Prefix: commonPrefix(b.keys, seg), Keys: b.keys},
			at:    b.firstIndex,
		})
	}

	sort.SliceStable(found, func(i, j int) bool { return found[i].at < found[j].at })
	for _, p := range found {
		groups = append(groups, p.group)
	}
	return groups, ungrouped
}

// leadingSegment returns everything up to and including the first underscore.
// A key without an underscore, or one that starts or ends with it, has no
// segment and can never form a group.
func leadingSegment(key string) (string, bool) {
	i := strings.Index(key, "_")
	if i <= 0 || i == len(key)-1 {
		return "", false
	}
	return key[:i+1], true
}

// commonPrefix extends seg to the longest prefix ending in "_" that every key
// shares, falling back to seg itself.
func commonPrefix(keys []string, seg string) string {
	if len(keys) == 0 {
		return seg
	}

	prefix := keys[0]
	for _, k := range keys[1:] {
		prefix = sharedPrefix(prefix, k)
		if len(prefix) <= len(seg) {
			return seg
		}
	}

	// Cut back to an underscore boundary; a prefix must not split a word.
	if i := strings.LastIndex(prefix, "_"); i >= 0 && i+1 > len(seg) {
		return prefix[:i+1]
	}
	return seg
}

func sharedPrefix(a, b string) string {
	n := min(len(a), len(b))
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return a[:i]
}

// NormalizeGroups brings group membership back in line with the keys after an
// edit, in place.
//
// Grouping was previously decided once, at import, and never revisited.
// Deleting members until only one was left produced a "group" of one, and two
// keys that clearly belonged together stayed apart because they happened to be
// added separately. The rule -- a prefix is a group once at least
// MinGroupSize keys share it -- now holds after every change, in both
// directions.
//
// A group that still has enough members keeps the prefix it has, even where
// DetectGroups would pick a longer one. That prefix may have been chosen
// deliberately, and re-deriving it would silently rewrite the key names shown
// in the editor.
func NormalizeGroups(entries []Entry) {
	// A prefix that no longer matches its key is stale -- the key was renamed
	// out of the group -- so it does not count towards the group's size.
	members := make(map[string]int)
	for _, e := range entries {
		if e.GroupPrefix != "" && strings.HasPrefix(e.Key, e.GroupPrefix) {
			members[e.GroupPrefix]++
		}
	}

	surviving := make(map[string]bool)
	for prefix, n := range members {
		if n >= MinGroupSize {
			surviving[prefix] = true
		}
	}

	// Dissolve everything that is not a group any more.
	for i := range entries {
		p := entries[i].GroupPrefix
		if p == "" {
			continue
		}
		if !surviving[p] || !strings.HasPrefix(entries[i].Key, p) {
			entries[i].GroupPrefix = ""
		}
	}

	// A loose key that fits an existing group joins it. Two keys sharing a
	// prefix should not sit apart just because one arrived later.
	var loose []string
	for i := range entries {
		if entries[i].GroupPrefix != "" {
			continue
		}
		if p := longestSurvivingPrefix(surviving, entries[i].Key); p != "" {
			entries[i].GroupPrefix = p
			continue
		}
		loose = append(loose, entries[i].Key)
	}

	// Whatever is still loose can form new groups among itself.
	groups, _ := DetectGroups(loose)
	prefixByKey := make(map[string]string)
	for _, g := range groups {
		for _, k := range g.Keys {
			prefixByKey[k] = g.Prefix
		}
	}
	for i := range entries {
		if entries[i].GroupPrefix == "" {
			entries[i].GroupPrefix = prefixByKey[entries[i].Key]
		}
	}
}

// longestSurvivingPrefix picks the most specific existing group a key fits.
func longestSurvivingPrefix(surviving map[string]bool, key string) string {
	best := ""
	for prefix := range surviving {
		if strings.HasPrefix(key, prefix) && len(prefix) > len(best) {
			best = prefix
		}
	}
	return best
}
