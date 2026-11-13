package mwanachamagit

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func mtEntry(name string) mergeEntry {
	return mergeEntry{BlobID: "blob-" + name, SHA: "sha-" + name}
}

func mtTree(entries map[string]string) mergeTree {
	tree := mergeTree{}
	for path, name := range entries {
		tree[path] = mtEntry(name)
	}
	return tree
}

func mtSameStrings(got, want []string) bool {
	if len(got) == 0 && len(want) == 0 {
		return true
	}
	return reflect.DeepEqual(got, want)
}

type mtCase struct {
	name            string
	base            map[string]string
	ours            map[string]string
	theirs          map[string]string
	wantMerged      map[string]string
	wantOverlapping []string
}

func TestMergeTrees(t *testing.T) {
	cases := []mtCase{
		{
			name:       "everything empty",
			wantMerged: map[string]string{},
		},
		{
			name:       "both sides identical to base",
			base:       map[string]string{"a": "a1", "b": "b1"},
			ours:       map[string]string{"a": "a1", "b": "b1"},
			theirs:     map[string]string{"a": "a1", "b": "b1"},
			wantMerged: map[string]string{"a": "a1", "b": "b1"},
		},
		{
			name:       "adds on different paths from both sides",
			base:       map[string]string{"keep": "k1"},
			ours:       map[string]string{"keep": "k1", "ours.txt": "o1"},
			theirs:     map[string]string{"keep": "k1", "theirs.txt": "t1"},
			wantMerged: map[string]string{"keep": "k1", "ours.txt": "o1", "theirs.txt": "t1"},
		},
		{
			name:       "edits on different paths from both sides",
			base:       map[string]string{"a": "a1", "b": "b1", "c": "c1"},
			ours:       map[string]string{"a": "a2", "b": "b1", "c": "c1"},
			theirs:     map[string]string{"a": "a1", "b": "b2", "c": "c1"},
			wantMerged: map[string]string{"a": "a2", "b": "b2", "c": "c1"},
		},
		{
			name:       "deletes on different paths from both sides",
			base:       map[string]string{"a": "a1", "b": "b1", "c": "c1"},
			ours:       map[string]string{"b": "b1", "c": "c1"},
			theirs:     map[string]string{"a": "a1", "c": "c1"},
			wantMerged: map[string]string{"c": "c1"},
		},
		{
			name:       "an add, an edit and a delete spread across both sides",
			base:       map[string]string{"edit": "e1", "gone": "g1", "same": "s1"},
			ours:       map[string]string{"edit": "e2", "same": "s1", "added": "n1"},
			theirs:     map[string]string{"edit": "e1", "same": "s1", "gone": "g1", "theirs-added": "n2"},
			wantMerged: map[string]string{"edit": "e2", "same": "s1", "added": "n1", "theirs-added": "n2"},
		},
		{
			name:       "ours unchanged takes every theirs change",
			base:       map[string]string{"a": "a1", "b": "b1", "c": "c1"},
			ours:       map[string]string{"a": "a1", "b": "b1", "c": "c1"},
			theirs:     map[string]string{"a": "a2", "c": "c1", "d": "d1"},
			wantMerged: map[string]string{"a": "a2", "c": "c1", "d": "d1"},
		},
		{
			name:       "theirs unchanged takes every ours change",
			base:       map[string]string{"a": "a1", "b": "b1", "c": "c1"},
			ours:       map[string]string{"a": "a2", "c": "c1", "d": "d1"},
			theirs:     map[string]string{"a": "a1", "b": "b1", "c": "c1"},
			wantMerged: map[string]string{"a": "a2", "c": "c1", "d": "d1"},
		},
		{
			name:       "identical edit on both sides agrees",
			base:       map[string]string{"a": "a1"},
			ours:       map[string]string{"a": "a2"},
			theirs:     map[string]string{"a": "a2"},
			wantMerged: map[string]string{"a": "a2"},
		},
		{
			name:       "both sides delete the same path agrees",
			base:       map[string]string{"a": "a1", "b": "b1"},
			ours:       map[string]string{"b": "b1"},
			theirs:     map[string]string{"b": "b1"},
			wantMerged: map[string]string{"b": "b1"},
		},
		{
			name:       "both sides add the same path with the same content agrees",
			base:       map[string]string{},
			ours:       map[string]string{"new": "n1"},
			theirs:     map[string]string{"new": "n1"},
			wantMerged: map[string]string{"new": "n1"},
		},
		{
			name:            "both sides add the same path with different content overlap",
			base:            map[string]string{"keep": "k1"},
			ours:            map[string]string{"keep": "k1", "new": "n-ours"},
			theirs:          map[string]string{"keep": "k1", "new": "n-theirs"},
			wantMerged:      map[string]string{"keep": "k1", "new": "n-theirs"},
			wantOverlapping: []string{"new"},
		},
		{
			name:            "both sides edit the same path differently overlap",
			base:            map[string]string{"a": "a1"},
			ours:            map[string]string{"a": "a-ours"},
			theirs:          map[string]string{"a": "a-theirs"},
			wantMerged:      map[string]string{"a": "a-theirs"},
			wantOverlapping: []string{"a"},
		},
		{
			name:            "ours edits and theirs deletes overlap and theirs deletion wins",
			base:            map[string]string{"a": "a1", "b": "b1"},
			ours:            map[string]string{"a": "a2", "b": "b1"},
			theirs:          map[string]string{"b": "b1"},
			wantMerged:      map[string]string{"b": "b1"},
			wantOverlapping: []string{"a"},
		},
		{
			name:            "ours deletes and theirs edits overlap and theirs edit wins",
			base:            map[string]string{"a": "a1", "b": "b1"},
			ours:            map[string]string{"b": "b1"},
			theirs:          map[string]string{"a": "a2", "b": "b1"},
			wantMerged:      map[string]string{"a": "a2", "b": "b1"},
			wantOverlapping: []string{"a"},
		},
		{
			name:       "empty base with disjoint adds",
			base:       map[string]string{},
			ours:       map[string]string{"o": "o1"},
			theirs:     map[string]string{"t": "t1"},
			wantMerged: map[string]string{"o": "o1", "t": "t1"},
		},
		{
			name:            "empty base with the same path added differently",
			base:            map[string]string{},
			ours:            map[string]string{"x": "x-ours", "o": "o1"},
			theirs:          map[string]string{"x": "x-theirs", "t": "t1"},
			wantMerged:      map[string]string{"x": "x-theirs", "o": "o1", "t": "t1"},
			wantOverlapping: []string{"x"},
		},
		{
			name:       "nil base behaves as empty",
			ours:       map[string]string{"o": "o1"},
			theirs:     map[string]string{"t": "t1"},
			wantMerged: map[string]string{"o": "o1", "t": "t1"},
		},
		{
			name:       "only ours has content against an empty base",
			base:       map[string]string{},
			ours:       map[string]string{"a": "a1", "b": "b1"},
			theirs:     map[string]string{},
			wantMerged: map[string]string{"a": "a1", "b": "b1"},
		},
		{
			name:       "theirs deleting everything against an unchanged ours",
			base:       map[string]string{"a": "a1", "b": "b1"},
			ours:       map[string]string{"a": "a1", "b": "b1"},
			theirs:     map[string]string{},
			wantMerged: map[string]string{},
		},
		{
			name:            "overlapping paths are returned sorted",
			base:            map[string]string{"z": "z1", "m": "m1", "a": "a1"},
			ours:            map[string]string{"z": "z-ours", "m": "m-ours", "a": "a-ours", "q": "q-ours"},
			theirs:          map[string]string{"z": "z-theirs", "m": "m-theirs", "a": "a-theirs", "q": "q-theirs"},
			wantMerged:      map[string]string{"z": "z-theirs", "m": "m-theirs", "a": "a-theirs", "q": "q-theirs"},
			wantOverlapping: []string{"a", "m", "q", "z"},
		},
		{
			name:            "clean paths never appear beside overlapping ones",
			base:            map[string]string{"clean-ours": "c1", "clean-theirs": "d1", "clash": "x1"},
			ours:            map[string]string{"clean-ours": "c2", "clean-theirs": "d1", "clash": "x-ours"},
			theirs:          map[string]string{"clean-ours": "c1", "clean-theirs": "d2", "clash": "x-theirs"},
			wantMerged:      map[string]string{"clean-ours": "c2", "clean-theirs": "d2", "clash": "x-theirs"},
			wantOverlapping: []string{"clash"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			merged, overlapping := mergeTrees(mtTree(tc.base), mtTree(tc.ours), mtTree(tc.theirs))
			want := mtTree(tc.wantMerged)
			if len(merged) != len(want) || (len(want) > 0 && !reflect.DeepEqual(merged, want)) {
				t.Fatalf("merged = %+v, want %+v", merged, want)
			}
			if !mtSameStrings(overlapping, tc.wantOverlapping) {
				t.Fatalf("overlapping = %v, want %v", overlapping, tc.wantOverlapping)
			}
			if !sort.StringsAreSorted(overlapping) {
				t.Fatalf("overlapping not sorted: %v", overlapping)
			}
		})
	}
}

func TestMergeTrees_DoesNotChangeItsInputs(t *testing.T) {
	base := mtTree(map[string]string{"a": "a1", "b": "b1"})
	ours := mtTree(map[string]string{"a": "a2", "b": "b1"})
	theirs := mtTree(map[string]string{"a": "a3", "c": "c1"})
	baseCopy, oursCopy, theirsCopy := mtTree(map[string]string{"a": "a1", "b": "b1"}),
		mtTree(map[string]string{"a": "a2", "b": "b1"}),
		mtTree(map[string]string{"a": "a3", "c": "c1"})

	mergeTrees(base, ours, theirs)

	if !reflect.DeepEqual(base, baseCopy) || !reflect.DeepEqual(ours, oursCopy) || !reflect.DeepEqual(theirs, theirsCopy) {
		t.Fatalf("inputs changed: base=%+v ours=%+v theirs=%+v", base, ours, theirs)
	}
}

type mbFixture struct {
	m    *gitManager
	repo models.Repository
	main models.Branch
}

func newMbFixture(t *testing.T) mbFixture {
	t.Helper()
	m := newTestManager(t)
	repo, main := mbRepo(t, m, "merge-base")
	return mbFixture{m: m, repo: repo, main: main}
}

func mbRepo(t *testing.T, m *gitManager, name string) (models.Repository, models.Branch) {
	t.Helper()
	ctx := context.Background()
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: name})
	if err != nil {
		t.Fatalf("InitRepo %s: %v", name, err)
	}
	branches, err := m.ListBranches(ctx, repo.ID)
	if err != nil || len(branches) == 0 {
		t.Fatalf("ListBranches %s: %+v (err=%v)", name, branches, err)
	}
	return repo, branches[0]
}

func (f mbFixture) commit(t *testing.T, branchID, path, content string) string {
	t.Helper()
	return mbCommit(t, f.m, branchID, path, content)
}

func mbCommit(t *testing.T, m *gitManager, branchID, path, content string) string {
	t.Helper()
	commit, err := m.WriteFile(context.Background(), WriteFileRequest{BranchID: branchID, Path: path, Content: content})
	if err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
	return commit.ID
}

func (f mbFixture) branch(t *testing.T, name, fromBranchID string) models.Branch {
	t.Helper()
	branch, err := f.m.CreateBranch(context.Background(), CreateBranchRequest{
		RepositoryID: f.repo.ID,
		Name:         name,
		FromBranchID: fromBranchID,
	})
	if err != nil {
		t.Fatalf("CreateBranch %s: %v", name, err)
	}
	return branch
}

func (f mbFixture) mergeCommit(t *testing.T, parentIDs ...string) string {
	t.Helper()
	ctx := context.Background()
	row := models.Commit{
		RepositoryID: f.repo.ID,
		SHA:          "merge-of-" + parentIDs[0],
		Message:      "merge",
		CreatedAt:    models.NowRFC3339(),
	}
	ensureID(&row)
	if err := f.m.db.WithContext(ctx).Table(f.m.tables.Commits).Create(&row).Error; err != nil {
		t.Fatalf("create merge commit: %v", err)
	}
	for i, parentID := range parentIDs {
		link := models.CommitParent{CommitID: row.ID, ParentID: parentID, ParentIndex: i}
		if err := f.m.db.WithContext(ctx).Table(f.m.tables.CommitParents).Create(&link).Error; err != nil {
			t.Fatalf("link merge commit to parent %s: %v", parentID, err)
		}
	}
	return row.ID
}

func (f mbFixture) assertBase(t *testing.T, commitA, commitB, want string) {
	t.Helper()
	ctx := context.Background()
	got, err := f.m.mergeBase(ctx, commitA, commitB)
	if err != nil {
		t.Fatalf("mergeBase(%s, %s): %v", commitA, commitB, err)
	}
	if got != want {
		t.Fatalf("mergeBase(%s, %s) = %q, want %q", commitA, commitB, got, want)
	}
	reversed, err := f.m.mergeBase(ctx, commitB, commitA)
	if err != nil {
		t.Fatalf("mergeBase(%s, %s): %v", commitB, commitA, err)
	}
	if reversed != want {
		t.Fatalf("mergeBase(%s, %s) = %q, want %q (argument order must not matter)", commitB, commitA, reversed, want)
	}
}

func TestMergeBase_LinearHistoryReturnsTheAncestor(t *testing.T) {
	f := newMbFixture(t)
	a := f.commit(t, f.main.ID, "a.txt", "one")
	f.commit(t, f.main.ID, "b.txt", "two")
	c := f.commit(t, f.main.ID, "c.txt", "three")

	f.assertBase(t, a, c, a)
}

func TestMergeBase_LinearHistoryAdjacentCommits(t *testing.T) {
	f := newMbFixture(t)
	a := f.commit(t, f.main.ID, "a.txt", "one")
	b := f.commit(t, f.main.ID, "b.txt", "two")

	f.assertBase(t, a, b, a)
}

func TestMergeBase_DivergedFromOneForkPoint(t *testing.T) {
	f := newMbFixture(t)
	f.commit(t, f.main.ID, "root.txt", "root")
	fork := f.commit(t, f.main.ID, "fork.txt", "fork point")
	feature := f.branch(t, "feature", f.main.ID)
	f.commit(t, f.main.ID, "main-1.txt", "main one")
	mainHead := f.commit(t, f.main.ID, "main-2.txt", "main two")
	f.commit(t, feature.ID, "feature-1.txt", "feature one")
	featureHead := f.commit(t, feature.ID, "feature-2.txt", "feature two")

	f.assertBase(t, mainHead, featureHead, fork)
}

func TestMergeBase_DivergedWithOneCommitEachSide(t *testing.T) {
	f := newMbFixture(t)
	fork := f.commit(t, f.main.ID, "base.txt", "base")
	feature := f.branch(t, "feature", f.main.ID)
	mainHead := f.commit(t, f.main.ID, "d.txt", "from default")
	featureHead := f.commit(t, feature.ID, "s.txt", "from source")

	f.assertBase(t, mainHead, featureHead, fork)
}

func TestMergeBase_SiblingBranchesFromTheSameHead(t *testing.T) {
	f := newMbFixture(t)
	head := f.commit(t, f.main.ID, "base.txt", "base")
	left := f.branch(t, "left", f.main.ID)
	right := f.branch(t, "right", f.main.ID)
	leftHead := f.commit(t, left.ID, "left.txt", "left")
	rightHead := f.commit(t, right.ID, "right.txt", "right")

	f.assertBase(t, leftHead, rightHead, head)
}

func TestMergeBase_SiblingBranchesBothAheadOfMain(t *testing.T) {
	f := newMbFixture(t)
	f.commit(t, f.main.ID, "base.txt", "base")
	head := f.commit(t, f.main.ID, "base-2.txt", "base two")
	left := f.branch(t, "left", f.main.ID)
	right := f.branch(t, "right", f.main.ID)
	f.commit(t, left.ID, "left-1.txt", "left one")
	leftHead := f.commit(t, left.ID, "left-2.txt", "left two")
	rightHead := f.commit(t, right.ID, "right-1.txt", "right one")

	f.assertBase(t, leftHead, rightHead, head)
	f.assertBase(t, leftHead, head, head)
}

func TestMergeBase_CommitWithItself(t *testing.T) {
	f := newMbFixture(t)
	f.commit(t, f.main.ID, "a.txt", "one")
	head := f.commit(t, f.main.ID, "b.txt", "two")

	f.assertBase(t, head, head, head)
}

func TestMergeBase_RootCommitWithItself(t *testing.T) {
	f := newMbFixture(t)
	root := f.commit(t, f.main.ID, "a.txt", "one")

	f.assertBase(t, root, root, root)
}

func TestMergeBase_MergeCommitWithOneOfItsParents(t *testing.T) {
	f := newMbFixture(t)
	base := f.commit(t, f.main.ID, "base.txt", "base")
	feature := f.branch(t, "feature", f.main.ID)
	mainSide := f.commit(t, f.main.ID, "main.txt", "main side")
	featureSide := f.commit(t, feature.ID, "feature.txt", "feature side")
	merged := f.mergeCommit(t, mainSide, featureSide)

	f.assertBase(t, merged, mainSide, mainSide)
	f.assertBase(t, merged, featureSide, featureSide)
	f.assertBase(t, merged, base, base)
}

func TestMergeBase_AfterAMergeCommitTheOtherSideIsTheBase(t *testing.T) {
	f := newMbFixture(t)
	f.commit(t, f.main.ID, "base.txt", "base")
	feature := f.branch(t, "feature", f.main.ID)
	mainSide := f.commit(t, f.main.ID, "main.txt", "main side")
	featureSide := f.commit(t, feature.ID, "feature.txt", "feature side")
	merged := f.mergeCommit(t, mainSide, featureSide)
	laterOnFeature := f.commit(t, feature.ID, "feature-2.txt", "feature later")

	f.assertBase(t, merged, laterOnFeature, featureSide)
}

func TestMergeBase_MergeCommitReachesBothLines(t *testing.T) {
	f := newMbFixture(t)
	base := f.commit(t, f.main.ID, "base.txt", "base")
	left := f.branch(t, "left", f.main.ID)
	right := f.branch(t, "right", f.main.ID)
	leftHead := f.commit(t, left.ID, "left.txt", "left")
	rightHead := f.commit(t, right.ID, "right.txt", "right")
	merged := f.mergeCommit(t, leftHead, rightHead)
	other := f.branch(t, "other", f.main.ID)
	otherHead := f.commit(t, other.ID, "other.txt", "other")

	f.assertBase(t, merged, otherHead, base)
}

func TestMergeBase_NoCommonAncestorReturnsEmpty(t *testing.T) {
	f := newMbFixture(t)
	first := f.commit(t, f.main.ID, "a.txt", "one")
	_, otherMain := mbRepo(t, f.m, "unrelated")
	second := mbCommit(t, f.m, otherMain.ID, "b.txt", "two")

	f.assertBase(t, first, second, "")
}

func TestMergeBase_NoCommonAncestorBetweenSeparateRoots(t *testing.T) {
	f := newMbFixture(t)
	f.commit(t, f.main.ID, "a.txt", "one")
	tip := f.commit(t, f.main.ID, "a2.txt", "one more")
	_, otherMain := mbRepo(t, f.m, "unrelated")
	mbCommit(t, f.m, otherMain.ID, "b.txt", "two")
	otherTip := mbCommit(t, f.m, otherMain.ID, "b2.txt", "two more")

	f.assertBase(t, tip, otherTip, "")
}
