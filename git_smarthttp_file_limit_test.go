package mwanachamagit

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

func blockingHandler(entered chan<- struct{}, release <-chan struct{}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		w.WriteHeader(http.StatusOK)
	})
}

func serveAsync(h http.Handler, wg *sync.WaitGroup, codes chan<- int) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/info/refs", nil))
		codes <- rec.Code
	}()
}

func TestGitRequestLimiter_ARequestOverTheLimitWaitsForASlotAndIsThenServed(t *testing.T) {
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	h := newRequestLimiter(2, 5*time.Second).wrap(blockingHandler(entered, release))
	var wg sync.WaitGroup
	codes := make(chan int, 3)
	for i := 0; i < 2; i++ {
		serveAsync(h, &wg, codes)
		<-entered
	}
	serveAsync(h, &wg, codes)
	select {
	case <-entered:
		t.Fatal("a third request was served while two held the slots")
	case <-time.After(150 * time.Millisecond):
	}
	release <- struct{}{}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the waiting request was not served after a slot was freed")
	}
	close(release)
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != http.StatusOK {
			t.Errorf("status = %d, want 200 for every request", code)
		}
	}
}

func TestGitRequestLimiter_ARequestThatWaitsTooLongAnswers503WithRetryAfter(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	h := newRequestLimiter(1, 100*time.Millisecond).wrap(blockingHandler(entered, release))
	var wg sync.WaitGroup
	codes := make(chan int, 1)
	serveAsync(h, &wg, codes)
	<-entered

	rec := httptest.NewRecorder()
	began := time.Now()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/info/refs", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
	if after, err := strconv.Atoi(rec.Header().Get("Retry-After")); err != nil || after <= 0 {
		t.Errorf("Retry-After = %q, want a positive number of seconds", rec.Header().Get("Retry-After"))
	}
	if waited := time.Since(began); waited < 100*time.Millisecond {
		t.Errorf("answered after %s, want it to wait for the slot first", waited)
	}
	close(release)
	wg.Wait()
	if code := <-codes; code != http.StatusOK {
		t.Errorf("the request holding the slot answered %d", code)
	}
}

func TestGitRequestLimiter_AFinishedRequestFreesItsSlotEvenWhenTheHandlerPanics(t *testing.T) {
	limiter := newRequestLimiter(1, 100*time.Millisecond)
	panicking := limiter.wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	func() {
		defer func() { _ = recover() }()
		panicking.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	}()
	ok := limiter.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	rec := httptest.NewRecorder()
	ok.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d after a panicking request, want 200", rec.Code)
	}
}

func TestGitRequestLimit_ComesFromTheEnvironmentAndDefaultsToFour(t *testing.T) {
	cases := map[string]int{"": 4, "7": 7, "1": 1, "0": 4, "-3": 4, "many": 4}
	for value, want := range cases {
		t.Setenv(gitRequestLimitEnv, value)
		if got := gitRequestLimit(); got != want {
			t.Errorf("%s=%q: limit = %d, want %d", gitRequestLimitEnv, value, got, want)
		}
	}
}

func TestRepoFileGitHTTPLimit_TheFileHandlerSharesTheProcessWideLimiter(t *testing.T) {
	s := newFileGitServer(t)
	if cap(processGitRequests.slots) != gitRequestLimit() {
		t.Errorf("process limiter holds %d slots, want %d", cap(processGitRequests.slots), gitRequestLimit())
	}
	for i := 0; i < 3*cap(processGitRequests.slots); i++ {
		s.clone(t)
	}
	if got := len(processGitRequests.slots); got != 0 {
		t.Errorf("%d slots still held after every clone finished", got)
	}
}
