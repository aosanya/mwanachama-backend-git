package mwanachamagit

import (
	"reflect"
	"sort"
	"testing"
)

const (
	featuresRepoFile      = "repofile_features_repo_test.go"
	featuresContentFile   = "repofile_features_content_test.go"
	featuresTransportFile = "repofile_features_transport_test.go"
	featuresCopyFile      = "repofile_features_copy_test.go"
)

var repoFileFeatureInventory = map[string]string{
	"InitRepo":                featuresRepoFile,
	"ListRepositories":        featuresRepoFile,
	"ListRepositorySummaries": featuresRepoFile,
	"GetRepository":           featuresRepoFile,
	"GetRepositoryByName":     featuresRepoFile,
	"DeleteRepo":              featuresRepoFile,
	"PurgeRepo":               featuresRepoFile,

	"CreateBranch":         featuresRepoFile,
	"GetBranch":            featuresRepoFile,
	"GetBranchByName":      featuresRepoFile,
	"ListBranches":         featuresRepoFile,
	"ListBranchesFiltered": featuresRepoFile,
	"DeleteBranch":         featuresRepoFile,
	"MergeBranch":          featuresRepoFile,
	"RebaseBranch":         featuresRepoFile,

	"CreateMergeRequest":   featuresRepoFile,
	"GetMergeRequest":      featuresRepoFile,
	"ListMergeRequests":    featuresRepoFile,
	"CompleteMergeRequest": featuresRepoFile,
	"CloseMergeRequest":    featuresRepoFile,

	"CreateTag": featuresRepoFile,
	"GetTag":    featuresRepoFile,
	"ListTags":  featuresRepoFile,
	"DeleteTag": featuresRepoFile,

	"WriteFile":     featuresContentFile,
	"ReadFile":      featuresContentFile,
	"DeleteFile":    featuresContentFile,
	"ListDirectory": featuresContentFile,

	"Log":  featuresContentFile,
	"Diff": featuresContentFile,

	"RollbackByWorkflowRun": featuresContentFile,

	"CreateKeyword":    featuresContentFile,
	"GetKeyword":       featuresContentFile,
	"ListKeywords":     featuresContentFile,
	"GetKeywordTree":   featuresContentFile,
	"UpdateKeyword":    featuresContentFile,
	"DeleteKeyword":    featuresContentFile,
	"SearchByKeywords": featuresContentFile,

	"CreateEdge":      featuresContentFile,
	"DeleteEdge":      featuresContentFile,
	"GetNeighborhood": featuresContentFile,
	"QueryGraph":      featuresContentFile,

	"SearchBlobs": featuresContentFile,

	"ImportRepo":      featuresTransportFile,
	"GetImportStatus": featuresTransportFile,
	"CancelImport":    featuresTransportFile,

	"FetchBranch":          featuresTransportFile,
	"GetFetchBranchStatus": featuresTransportFile,
	"IndexPushedBranch":    featuresTransportFile,
	"SmartHTTPHandler":     featuresTransportFile,
}

func TestRepoFileFeatures_Inventory(t *testing.T) {
	managerType := reflect.TypeOf((*GitManager)(nil)).Elem()

	declared := make(map[string]bool, managerType.NumMethod())
	for i := 0; i < managerType.NumMethod(); i++ {
		declared[managerType.Method(i).Name] = true
	}

	var missing []string
	for name := range declared {
		if _, ok := repoFileFeatureInventory[name]; !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	for _, name := range missing {
		t.Errorf("GitManager.%s has no entry in repoFileFeatureInventory; add it and a test in one of the repofile_features_*_test.go files", name)
	}

	var unknown []string
	for name := range repoFileFeatureInventory {
		if !declared[name] {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	for _, name := range unknown {
		t.Errorf("repoFileFeatureInventory names %s, which GitManager does not declare", name)
	}

	allowedFiles := map[string]bool{
		featuresRepoFile:      true,
		featuresContentFile:   true,
		featuresTransportFile: true,
		featuresCopyFile:      true,
	}
	for name, file := range repoFileFeatureInventory {
		if !allowedFiles[file] {
			t.Errorf("%s is assigned to %q, which is not one of the four feature test files", name, file)
		}
	}
}
