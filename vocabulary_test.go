package mwanachamagit

import (
	"sort"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/models"
)

func vocabulary() map[string][2]any {
	return map[string][2]any{
		roleMergeRequest + ".status": {
			[]string{
				models.MergeRequestStatusOpen,
				models.MergeRequestStatusMerged,
				models.MergeRequestStatusClosed,
				models.MergeRequestStatusFailed,
				models.MergeRequestStatusRolledBack,
			}, "status",
		},
		roleImportJob + ".status": {
			[]string{
				models.ImportStatusPending,
				models.ImportStatusRunning,
				models.ImportStatusCompleted,
				models.ImportStatusFailed,
				models.ImportStatusCancelled,
			}, "status",
		},
		roleFetchBranchJob + ".status": {
			[]string{
				models.FetchJobStatusPending,
				models.FetchJobStatusRunning,
				models.FetchJobStatusCompleted,
				models.FetchJobStatusFailed,
			}, "status",
		},
		roleBlobReference + ".name": {
			[]string{
				models.EdgeTaggedWith,
				models.EdgeReferences,
				models.EdgeReferencedBy,
				models.EdgeDocuments,
				models.EdgeDocumentedBy,
				models.EdgeDependsOn,
				models.EdgeImportedBy,
			}, "name",
		},
	}
}

func TestVocabularyMatchesTheBlueprint(t *testing.T) {
	b, err := Blueprint()
	if err != nil {
		t.Fatalf("blueprint: %v", err)
	}

	for key, pair := range vocabulary() {
		role := key[:len(key)-len("."+pair[1].(string))]
		goValues := pair[0].([]string)
		field := pair[1].(string)

		var declared []string
		var found bool
		for _, o := range b.Objects {
			if o.Role != role {
				continue
			}
			for _, f := range o.Fields {
				if f.Name == field {
					declared, found = f.Values, true
				}
			}
		}
		if !found {
			t.Errorf("%s: the blueprint declares no such field", key)
			continue
		}

		sort.Strings(goValues)
		sorted := append([]string(nil), declared...)
		sort.Strings(sorted)

		for _, v := range goValues {
			if !declares(sorted, v) {
				t.Errorf("%s: Go compares against %q, which the blueprint does not declare — a stored value outliving a rename", key, v)
			}
		}
		for _, v := range sorted {
			if !declares(goValues, v) {
				t.Errorf("%s: the blueprint declares %q, which no Go constant names", key, v)
			}
		}
	}
}

func TestEveryDeclaredEnumIsHeldByTheVocabulary(t *testing.T) {
	b, err := Blueprint()
	if err != nil {
		t.Fatalf("blueprint: %v", err)
	}
	held := vocabulary()
	for _, o := range b.Objects {
		for _, f := range o.Fields {
			if len(f.Values) == 0 {
				continue
			}
			if _, ok := held[o.Role+"."+f.Name]; !ok {
				t.Errorf("%s.%s declares values that no Go constant is held against; add it to vocabulary() or the two will drift", o.Role, f.Name)
			}
		}
	}
}
