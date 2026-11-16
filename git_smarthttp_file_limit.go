package mwanachamagit

import (
	"net/http"
	"os"
	"strconv"
	"time"
)

const (
	gitRequestLimitEnv     = "WAKALA_GIT_MAX_CONCURRENT_REQUESTS"
	defaultGitRequestLimit = 4
	gitRequestWait         = 15 * time.Second
	gitRetryAfterSeconds   = "5"
)

var processGitRequests = newRequestLimiter(gitRequestLimit(), gitRequestWait)

func gitRequestLimit() int {
	limit, err := strconv.Atoi(os.Getenv(gitRequestLimitEnv))
	if err != nil || limit < 1 {
		return defaultGitRequestLimit
	}
	return limit
}

type requestLimiter struct {
	slots chan struct{}
	wait  time.Duration
}

func newRequestLimiter(limit int, wait time.Duration) *requestLimiter {
	return &requestLimiter{slots: make(chan struct{}, limit), wait: wait}
}

func (l *requestLimiter) acquire(r *http.Request) bool {
	select {
	case l.slots <- struct{}{}:
		return true
	default:
	}
	timer := time.NewTimer(l.wait)
	defer timer.Stop()
	select {
	case l.slots <- struct{}{}:
		return true
	case <-timer.C:
		return false
	case <-r.Context().Done():
		return false
	}
}

func (l *requestLimiter) release() { <-l.slots }

func (l *requestLimiter) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.acquire(r) {
			w.Header().Set("Retry-After", gitRetryAfterSeconds)
			http.Error(w, "the hub is serving as many git requests as it can; try again shortly", http.StatusServiceUnavailable)
			return
		}
		defer l.release()
		next.ServeHTTP(w, r)
	})
}
