package mwanachamagit

import (
	"context"
	"testing"
)

func TestWriteFileSiblingPathsSharingNamePrefix(t *testing.T) {
	cases := []struct {
		name  string
		paths []string
	}{
		{"drawable-v21 then drawable", []string{
			"android/app/src/main/res/drawable-v21/launch_background.xml",
			"android/app/src/main/res/drawable/launch_background.xml",
		}},
		{"values-night then values", []string{
			"android/app/src/main/res/values-night/styles.xml",
			"android/app/src/main/res/values/styles.xml",
		}},
		{"gradle.properties then gradle", []string{
			"android/gradle.properties",
			"android/gradle/wrapper/gradle-wrapper.properties",
		}},
	}
	for _, sameContent := range []bool{true, false} {
		for _, tc := range cases {
			name := tc.name
			if sameContent {
				name += " same content"
			}
			t.Run(name, func(t *testing.T) {
				ctx := context.Background()
				m := newTestManager(t)
				repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "flutter-app"})
				if err != nil {
					t.Fatalf("InitRepo: %v", err)
				}
				branches, _ := m.ListBranches(ctx, repo.ID)
				branch := branches[0]

				contents := map[string]string{}
				for i, p := range tc.paths {
					content := "same"
					if !sameContent {
						content = "content " + string(rune('a'+i))
					}
					contents[p] = content
					if _, err := m.WriteFile(ctx, WriteFileRequest{BranchID: branch.ID, Path: p, Content: content}); err != nil {
						t.Fatalf("WriteFile %q: %v", p, err)
					}
				}
				for p, want := range contents {
					blob, err := m.ReadFile(ctx, branch.ID, p)
					if err != nil {
						t.Fatalf("ReadFile %q: %v", p, err)
					}
					if blob.Content != want {
						t.Fatalf("ReadFile %q: got %q want %q", p, blob.Content, want)
					}
				}
			})
		}
	}
}

func TestWriteFileSixSiblingFilesAllReadBack(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	repo, err := m.InitRepo(ctx, CreateRepoRequest{Name: "flutter-app"})
	if err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	branches, _ := m.ListBranches(ctx, repo.ID)
	branch := branches[0]

	paths := []string{
		"android/app/src/main/res/drawable-v21/launch_background.xml",
		"android/app/src/main/res/drawable/launch_background.xml",
		"android/app/src/main/res/values-night/styles.xml",
		"android/app/src/main/res/values/styles.xml",
		"android/gradle.properties",
		"android/gradle/wrapper/gradle-wrapper.properties",
	}
	for _, p := range paths {
		if _, err := m.WriteFile(ctx, WriteFileRequest{BranchID: branch.ID, Path: p, Content: "x"}); err != nil {
			t.Fatalf("WriteFile %q: %v", p, err)
		}
	}
	for _, p := range paths {
		blob, err := m.ReadFile(ctx, branch.ID, p)
		if err != nil {
			t.Fatalf("ReadFile %q: %v", p, err)
		}
		if blob.Content != "x" {
			t.Fatalf("ReadFile %q: got %q", p, blob.Content)
		}
	}
}
