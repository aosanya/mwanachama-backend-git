package mwanachamagit

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	bigFileCount = 6
	bigFileBytes = 10 << 20
	bigRepoBytes = bigFileCount * bigFileBytes
)

func bigText(seed int64, size int) string {
	rng := rand.New(rand.NewSource(seed))
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789 \n"
	var sb strings.Builder
	sb.Grow(size)
	for sb.Len() < size {
		sb.WriteByte(alphabet[rng.Intn(len(alphabet))])
	}
	return sb.String()
}

func newBigFileGitServer(t *testing.T) fileGitServer {
	t.Helper()
	transportRequireGit(t)
	rf, repo, mainBranch := newFeatureRepo(t)
	ctx := context.Background()
	for i := 0; i < bigFileCount; i++ {
		_, err := rf.WriteFile(ctx, WriteFileRequest{
			BranchID: mainBranch.ID,
			Path:     fmt.Sprintf("big/file-%d.txt", i),
			Content:  bigText(int64(i+1), bigFileBytes),
		})
		if err != nil {
			t.Fatalf("WriteFile big file %d: %v", i, err)
		}
	}
	srv := httptest.NewServer(rf.GitHTTPHandler())
	t.Cleanup(srv.Close)
	return fileGitServer{rf: rf, repo: repo.ID, main: mainBranch.ID, url: srv.URL}
}

type memoryUse struct {
	allocated uint64
	peak      uint64
}

func measureMemory(run func()) memoryUse {
	previous := debug.SetGCPercent(20)
	defer debug.SetGCPercent(previous)
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var peak uint64
	wg.Add(1)
	go func() {
		defer wg.Done()
		var ms runtime.MemStats
		for {
			runtime.ReadMemStats(&ms)
			if ms.HeapAlloc > peak {
				peak = ms.HeapAlloc
			}
			select {
			case <-stop:
				return
			case <-time.After(2 * time.Millisecond):
			}
		}
	}()

	run()
	close(stop)
	wg.Wait()

	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	grown := uint64(0)
	if peak > before.HeapAlloc {
		grown = peak - before.HeapAlloc
	}
	return memoryUse{allocated: after.TotalAlloc - before.TotalAlloc, peak: grown}
}

func mib(n uint64) string { return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20)) }

func TestRepoFileGitHTTPMemory_RefAdvertisementOfASixtyMegabyteRepositoryBuildsNoObjects(t *testing.T) {
	s := newBigFileGitServer(t)
	handler := s.rf.GitHTTPHandler()

	for _, service := range []string{"git-upload-pack", "git-receive-pack"} {
		var status int
		var body string
		use := measureMemory(func() {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/info/refs?service="+service, nil))
			status, body = rec.Code, rec.Body.String()
		})
		t.Logf("%s: allocated %s, peak heap growth %s for a %s repository", service, mib(use.allocated), mib(use.peak), mib(bigRepoBytes))
		if status != http.StatusOK {
			t.Fatalf("%s: status = %d", service, status)
		}
		if !strings.Contains(body, "refs/heads/main") {
			t.Errorf("%s: advertisement does not name main:\n%s", service, body)
		}
		if limit := uint64(8 << 20); use.allocated > limit {
			t.Errorf("%s: advertisement allocated %s, want at most %s", service, mib(use.allocated), mib(limit))
		}
	}
}
