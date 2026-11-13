package mwanachamagit

import (
	"context"
	"sort"
)

type mergeEntry struct{ BlobID, SHA string }

type mergeTree map[string]mergeEntry

func sameMergeState(a mergeEntry, aPresent bool, b mergeEntry, bPresent bool) bool {
	if aPresent != bPresent {
		return false
	}
	return !aPresent || a.SHA == b.SHA
}

func mergeTrees(base, ours, theirs mergeTree) (merged mergeTree, overlapping []string) {
	merged = mergeTree{}
	paths := map[string]struct{}{}
	for _, tree := range []mergeTree{base, ours, theirs} {
		for path := range tree {
			paths[path] = struct{}{}
		}
	}
	for path := range paths {
		b, inBase := base[path]
		o, inOurs := ours[path]
		t, inTheirs := theirs[path]

		var chosen mergeEntry
		var present bool
		switch {
		case sameMergeState(o, inOurs, b, inBase):
			chosen, present = t, inTheirs
		case sameMergeState(t, inTheirs, b, inBase):
			chosen, present = o, inOurs
		case sameMergeState(o, inOurs, t, inTheirs):
			chosen, present = o, inOurs
		default:
			overlapping = append(overlapping, path)
			chosen, present = t, inTheirs
		}
		if present {
			merged[path] = chosen
		}
	}
	sort.Strings(overlapping)
	return merged, overlapping
}

func (m *gitManager) mergeBase(ctx context.Context, commitA, commitB string) (string, error) {
	db := m.db.WithContext(ctx)
	chainA, err := CommitChainIDs(db, m.tables, commitA, 0)
	if err != nil {
		return "", err
	}
	ancestorsOfA := make(map[string]struct{}, len(chainA))
	for _, id := range chainA {
		ancestorsOfA[id] = struct{}{}
	}
	chainB, err := CommitChainIDs(db, m.tables, commitB, 0)
	if err != nil {
		return "", err
	}
	for _, id := range chainB {
		if _, ok := ancestorsOfA[id]; ok {
			return id, nil
		}
	}
	return "", nil
}
