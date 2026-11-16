package mwanachamagit

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestSearchBlobsWithEmptyQueryAnswersErrQueryRequired(t *testing.T) {
	m := newTestManager(t)
	_, err := m.SearchBlobs(context.Background(), SearchBlobsRequest{})
	if !errors.Is(err, ErrQueryRequired) {
		t.Fatalf("got %v, want ErrQueryRequired", err)
	}
}

func TestCreateMergeRequestWithoutTitleAnswersErrTitleRequired(t *testing.T) {
	m := newTestManager(t)
	_, err := m.CreateMergeRequest(context.Background(), CreateMergeRequestRequest{SourceBranchID: "b"})
	if !errors.Is(err, ErrTitleRequired) {
		t.Fatalf("got %v, want ErrTitleRequired", err)
	}
}

func TestCreateMergeRequestWithoutSourceBranchAnswersErrSourceBranchRequired(t *testing.T) {
	m := newTestManager(t)
	_, err := m.CreateMergeRequest(context.Background(), CreateMergeRequestRequest{Title: "t"})
	if !errors.Is(err, ErrSourceBranchRequired) {
		t.Fatalf("got %v, want ErrSourceBranchRequired", err)
	}
}

func TestFetchBranchOfMissingBranchAnswersErrBranchNotFound(t *testing.T) {
	m := newTestManager(t)
	_, err := m.FetchBranch(context.Background(), FetchBranchRequest{RepoID: "r", BranchID: "missing"})
	if !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("got %v, want ErrBranchNotFound", err)
	}
}

func TestInputErrorsAreDeclaredAs400(t *testing.T) {
	var declared struct {
		Errors map[string]int `json:"errors"`
	}
	if err := json.Unmarshal(OperationsJSON(), &declared); err != nil {
		t.Fatalf("operations json: %v", err)
	}
	sentinels := Sentinels()
	for _, name := range []string{"ErrQueryRequired", "ErrTitleRequired", "ErrSourceBranchRequired"} {
		if declared.Errors[name] != 400 {
			t.Errorf("%s declared as %d, want 400", name, declared.Errors[name])
		}
		if _, ok := sentinels[name]; !ok {
			t.Errorf("%s missing from Sentinels", name)
		}
	}
}
