package mwanachamagit

import "testing"

func assertMergeLines(t *testing.T, base, ours, theirs, wantMerged string, wantConflict bool) {
	t.Helper()
	merged, conflict := mergeLines(base, ours, theirs)
	if conflict != wantConflict {
		t.Fatalf("conflict = %v, want %v", conflict, wantConflict)
	}
	if !wantConflict && merged != wantMerged {
		t.Fatalf("merged = %q, want %q", merged, wantMerged)
	}
}

func TestMergeLines_OnlyOursEdits(t *testing.T) {
	assertMergeLines(t, "a\nb\nc\n", "a\nB\nc\n", "a\nb\nc\n", "a\nB\nc\n", false)
}

func TestMergeLines_OnlyTheirsEdits(t *testing.T) {
	assertMergeLines(t, "a\nb\nc\n", "a\nb\nc\n", "a\nb\nC\n", "a\nb\nC\n", false)
}

func TestMergeLines_TopAndBottomEditsMerge(t *testing.T) {
	assertMergeLines(t, "a\nb\nc\nd\ne\n", "A\nb\nc\nd\ne\n", "a\nb\nc\nd\nE\n", "A\nb\nc\nd\nE\n", false)
}

func TestMergeLines_SameLineDifferentEditsConflict(t *testing.T) {
	assertMergeLines(t, "a\nb\nc\n", "a\nX\nc\n", "a\nY\nc\n", "", true)
}

func TestMergeLines_IdenticalEditIsClean(t *testing.T) {
	assertMergeLines(t, "a\nb\nc\n", "a\nX\nc\n", "a\nX\nc\n", "a\nX\nc\n", false)
}

func TestMergeLines_AdjacentLineEditsConflict(t *testing.T) {
	assertMergeLines(t, "a\nb\nc\nd\n", "a\nB\nc\nd\n", "a\nb\nC\nd\n", "", true)
}

func TestMergeLines_SeparatedByOneLineEditsMerge(t *testing.T) {
	assertMergeLines(t, "a\nb\nc\nd\n", "A\nb\nc\nd\n", "a\nb\nC\nd\n", "A\nb\nC\nd\n", false)
}

func TestMergeLines_InsertDifferentTextAtSamePointConflicts(t *testing.T) {
	assertMergeLines(t, "a\nb\n", "a\nX\nb\n", "a\nY\nb\n", "", true)
}

func TestMergeLines_InsertIdenticalTextAtSamePointIsClean(t *testing.T) {
	assertMergeLines(t, "a\nb\n", "a\nX\nb\n", "a\nX\nb\n", "a\nX\nb\n", false)
}

func TestMergeLines_InsertsAtDistinctPointsMerge(t *testing.T) {
	assertMergeLines(t, "a\nb\nc\nd\ne\n", "a\nX\nb\nc\nd\ne\n", "a\nb\nc\nd\nY\ne\n", "a\nX\nb\nc\nd\nY\ne\n", false)
}

func TestMergeLines_OursDeletesTheirsLeavesIsClean(t *testing.T) {
	assertMergeLines(t, "a\nb\nc\n", "a\nc\n", "a\nb\nc\n", "a\nc\n", false)
}

func TestMergeLines_TheirsDeletesOursLeavesIsClean(t *testing.T) {
	assertMergeLines(t, "a\nb\nc\n", "a\nb\nc\n", "a\nc\n", "a\nc\n", false)
}

func TestMergeLines_DeleteAndDistantEditMerge(t *testing.T) {
	assertMergeLines(t, "a\nb\nc\nd\ne\n", "a\nc\nd\ne\n", "a\nb\nc\nd\nE\n", "a\nc\nd\nE\n", false)
}

func TestMergeLines_OursDeletesTheirsEditsConflicts(t *testing.T) {
	assertMergeLines(t, "a\nb\nc\n", "a\nc\n", "a\nB\nc\n", "", true)
}

func TestMergeLines_TheirsDeletesOursEditsConflicts(t *testing.T) {
	assertMergeLines(t, "a\nb\nc\n", "a\nB\nc\n", "a\nc\n", "", true)
}

func TestMergeLines_BothDeleteSameLinesIsClean(t *testing.T) {
	assertMergeLines(t, "a\nb\nc\n", "a\nc\n", "a\nc\n", "a\nc\n", false)
}

func TestMergeLines_TrailingNewlinePreserved(t *testing.T) {
	assertMergeLines(t, "a\nb\n", "a\nB\n", "a\nb\n", "a\nB\n", false)
}

func TestMergeLines_MissingTrailingNewlineKept(t *testing.T) {
	assertMergeLines(t, "a\nb", "A\nb", "a\nb", "A\nb", false)
}

func TestMergeLines_MissingTrailingNewlineBothEdits(t *testing.T) {
	assertMergeLines(t, "a\nb\nc", "A\nb\nc", "a\nb\nC", "A\nb\nC", false)
}

func TestMergeLines_TheirsAddsTrailingNewline(t *testing.T) {
	assertMergeLines(t, "a\nb", "A\nb", "a\nb\n", "A\nb\n", false)
}

func TestMergeLines_EmptyBaseDifferentContentsConflict(t *testing.T) {
	assertMergeLines(t, "", "x\n", "y\n", "", true)
}

func TestMergeLines_EmptyBaseIdenticalContentsClean(t *testing.T) {
	assertMergeLines(t, "", "x\n", "x\n", "x\n", false)
}

func TestMergeLines_EmptyBaseOnlyOursAdds(t *testing.T) {
	assertMergeLines(t, "", "x\n", "", "x\n", false)
}

func TestMergeLines_NulByteDifferingOnBothSidesConflicts(t *testing.T) {
	assertMergeLines(t, "a\x00b\n", "a\x00c\n", "a\x00d\n", "", true)
}
