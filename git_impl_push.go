package mwanachamagit

import (
	"context"
	"fmt"
)

// ErrPushIndexingNotImplemented is returned by [gitManager.IndexPushedBranch].
// See the method's doc comment for why.
var ErrPushIndexingNotImplemented = fmt.Errorf("IndexPushedBranch: not implemented (GIT task G6 — real git-push wire protocol)")

// IndexPushedBranch is a deliberate stub. The original CodeValdGit
// implementation (git_impl_index.go's syncGitGraph, called from the v1 Git
// Smart HTTP receive-pack handler) needs a real git-push wire-protocol
// listener and the dropped v1 Backend.OpenStorer abstraction, plus an
// internal .git-graph/ sync package that wasn't ported. CLAUDE.md already
// scopes "real git clone/fetch/push over the wire" out of mwanachama-git —
// mwanachama-kazi doesn't act as a git remote other tools push to — so unlike
// FetchBranch/ImportRepo (which only *originate* an outbound clone), this
// method has no client to satisfy yet. Revisit only if a real inbound push
// listener is confirmed needed (GIT task G6).
func (m *gitManager) IndexPushedBranch(ctx context.Context, repoName, branchRef, oldSHA, newSHA string) error {
	return ErrPushIndexingNotImplemented
}
