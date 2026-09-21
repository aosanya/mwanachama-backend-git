// git_smarthttp.go implements [GitManager.SmartHTTPHandler] — the real
// inbound git Smart HTTP wire protocol (G6). Ported from the original
// CodeValdGit implementation (internal/server/githttp.go), which served
// upload-pack and receive-pack for every agency's repo over its own
// entitygraph-backed Backend.OpenStorer; this port is single-tenant (this
// repo dropped the Agency concept entirely, see recent git log) and reads
// straight off the on-disk bare clone openOrInitBareRepo
// (git_impl_push.go) resolves — go-git's own transport/server package does
// every byte of wire-protocol/packfile work, this file only adapts it to
// net/http and, on a successful push, calls IndexPushedBranch.
package mwanachamagit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/pktline"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/capability"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gogitserver "github.com/go-git/go-git/v5/plumbing/transport/server"
)

// SmartHTTPHandler returns an http.Handler serving the git Smart HTTP
// protocol — see [GitManager]'s own doc for the URL shape a caller mounts
// this under.
func (m *gitManager) SmartHTTPHandler() http.Handler {
	return &smartHTTPHandler{m: m, srv: gogitserver.NewServer(&repoLoader{m: m})}
}

// smartHTTPHandler routes the three Smart HTTP endpoints. repoName is the
// URL's first path segment; everything after it selects which of the
// three.
type smartHTTPHandler struct {
	m   *gitManager
	srv transport.Transport
}

func (h *smartHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	repoName, rest, ok := splitRepoName(r.URL.Path)
	if !ok {
		http.Error(w, "invalid repository path", http.StatusBadRequest)
		return
	}

	switch {
	case r.Method == http.MethodGet && rest == "/info/refs":
		h.infoRefs(w, r, repoName)
	case r.Method == http.MethodPost && rest == "/git-upload-pack":
		h.uploadPack(w, r, repoName)
	case r.Method == http.MethodPost && rest == "/git-receive-pack":
		h.receivePack(w, r, repoName)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// infoRefs handles GET /{repoName}/info/refs?service=git-{upload,receive}-pack
// — the Smart HTTP service announcement followed by the advertised refs.
func (h *smartHTTPHandler) infoRefs(w http.ResponseWriter, r *http.Request, repoName string) {
	service := r.URL.Query().Get("service")
	if service != transport.UploadPackServiceName && service != transport.ReceivePackServiceName {
		http.Error(w, "unsupported service", http.StatusForbidden)
		return
	}

	ep, err := endpointFor(repoName)
	if err != nil {
		http.Error(w, "bad endpoint", http.StatusInternalServerError)
		return
	}

	var advRefs *packp.AdvRefs
	if service == transport.UploadPackServiceName {
		sess, err := h.srv.NewUploadPackSession(ep, nil)
		if err != nil {
			httpErrorFromTransport(w, err)
			return
		}
		defer sess.Close() //nolint:errcheck
		if advRefs, err = sess.AdvertisedReferencesContext(r.Context()); err != nil {
			httpErrorFromTransport(w, err)
			return
		}
		// Served by serveShallowClone; go-git's own session cannot.
		if err := advRefs.Capabilities.Set(capability.Shallow); err != nil {
			httpErrorFromTransport(w, err)
			return
		}
	} else {
		sess, err := h.srv.NewReceivePackSession(ep, nil)
		if err != nil {
			httpErrorFromTransport(w, err)
			return
		}
		defer sess.Close() //nolint:errcheck
		if advRefs, err = sess.AdvertisedReferencesContext(r.Context()); err != nil {
			httpErrorFromTransport(w, err)
			return
		}
	}

	// Prepend the Smart HTTP service header ("# service=git-…\n" + flush-pkt);
	// AdvRefs.Encode emits this prefix before the ref list itself.
	advRefs.Prefix = [][]byte{[]byte(fmt.Sprintf("# service=%s", service)), pktline.Flush}

	setNoCacheHeaders(w)
	w.Header().Set("Content-Type", fmt.Sprintf("application/x-git-%s-advertisement", strings.TrimPrefix(service, "git-")))
	w.WriteHeader(http.StatusOK)
	_ = advRefs.Encode(w)
}

// uploadPack handles POST /{repoName}/git-upload-pack (fetch/clone).
func (h *smartHTTPHandler) uploadPack(w http.ResponseWriter, r *http.Request, repoName string) {
	ep, err := endpointFor(repoName)
	if err != nil {
		http.Error(w, "bad endpoint", http.StatusInternalServerError)
		return
	}
	sess, err := h.srv.NewUploadPackSession(ep, nil)
	if err != nil {
		httpErrorFromTransport(w, err)
		return
	}
	defer sess.Close() //nolint:errcheck

	// Must be called before UploadPack to initialise go-git's session state.
	if _, err := sess.AdvertisedReferencesContext(r.Context()); err != nil {
		httpErrorFromTransport(w, err)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read upload-pack request: "+err.Error(), http.StatusBadRequest)
		return
	}
	req := packp.NewUploadPackRequest()
	if err := req.Decode(bytes.NewReader(body)); err != nil {
		http.Error(w, "malformed upload-pack request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !req.Depth.IsZero() || len(req.Shallows) > 0 {
		if why := shallowRefusal(req); why != "" {
			http.Error(w, "shallow request not supported: "+why, http.StatusNotImplemented)
			return
		}
		h.serveShallowClone(w, r.Context(), repoName, req, bytes.HasSuffix(body, []byte("0009done\n")))
		return
	}
	stripShallowCap(req)
	resp, err := sess.UploadPack(r.Context(), req)
	if err != nil {
		httpErrorFromTransport(w, err)
		return
	}

	setNoCacheHeaders(w)
	w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
	w.WriteHeader(http.StatusOK)
	_ = resp.Encode(w)
}

// receivePack handles POST /{repoName}/git-receive-pack (push) — on
// success, calls IndexPushedBranch for every non-delete ref update,
// synchronously (the caller already returns as soon as ServeHTTP does;
// this keeps IndexPushedBranch failures visible in the same request/log
// rather than a detached goroutine nothing awaits — G6 has no queue to
// hand these off to yet).
func (h *smartHTTPHandler) receivePack(w http.ResponseWriter, r *http.Request, repoName string) {
	ep, err := endpointFor(repoName)
	if err != nil {
		http.Error(w, "bad endpoint", http.StatusInternalServerError)
		return
	}
	sess, err := h.srv.NewReceivePackSession(ep, nil)
	if err != nil {
		httpErrorFromTransport(w, err)
		return
	}
	defer sess.Close() //nolint:errcheck

	if _, err := sess.AdvertisedReferencesContext(r.Context()); err != nil {
		httpErrorFromTransport(w, err)
		return
	}

	req := packp.NewReferenceUpdateRequest()
	if err := req.Decode(r.Body); err != nil {
		http.Error(w, "malformed receive-pack request: "+err.Error(), http.StatusBadRequest)
		return
	}

	// go-git's ReceivePack requires a packfile and fails with "empty
	// packfile" when every command is a ref deletion (e.g. `git push
	// --delete`) — there are no objects to transfer for a delete. Handle
	// that case directly against the storer instead of going through
	// ReceivePack at all.
	if isDeleteOnly(req) {
		h.receivePackDeletes(w, r.Context(), repoName, req)
		return
	}

	status, err := sess.ReceivePack(r.Context(), req)
	if err != nil {
		log.Printf("[receive-pack] repo=%q: ReceivePack error: %v", repoName, err)
		httpErrorFromTransport(w, err)
		return
	}

	for _, cmd := range req.Commands {
		if cmd == nil || cmd.New.IsZero() {
			continue // a delete — nothing to index
		}
		if idxErr := h.m.IndexPushedBranch(context.Background(), repoName, cmd.Name.String(), cmd.Old.String(), cmd.New.String()); idxErr != nil {
			log.Printf("[receive-pack] repo=%q ref=%s: IndexPushedBranch failed (push already accepted; ref updated, index did not catch up): %v",
				repoName, cmd.Name, idxErr)
		}
	}
	h.settleHEAD(r.Context(), repoName)

	setNoCacheHeaders(w)
	w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
	w.WriteHeader(http.StatusOK)
	_ = status.Encode(w)
}

// settleHEAD repoints the bare repo's HEAD if the push left it dangling, so
// the next `git clone` has something to check out — best-effort, and logged
// rather than failed, since the push itself has already been accepted and
// the refs are already updated.
func (h *smartHTTPHandler) settleHEAD(ctx context.Context, repoName string) {
	repo, err := h.m.openOrInitBareRepo(ctx, repoName)
	if err != nil {
		log.Printf("[receive-pack] repo=%q: settleHEAD: open: %v", repoName, err)
		return
	}
	var preferred string
	if repoRow, err := h.m.GetRepositoryByName(ctx, repoName); err == nil {
		preferred = repoRow.DefaultBranch
	}
	moved, err := ensureHEADResolves(repo, preferred)
	if err != nil {
		log.Printf("[receive-pack] repo=%q: settleHEAD: %v", repoName, err)
		return
	}
	if moved {
		log.Printf("[receive-pack] repo=%q: HEAD was dangling, repointed at an existing branch", repoName)
	}
}

// isDeleteOnly reports whether every command in req deletes a ref (New is
// the zero hash) — see receivePack's own doc for why this needs its own
// path.
func isDeleteOnly(req *packp.ReferenceUpdateRequest) bool {
	if len(req.Commands) == 0 {
		return false
	}
	for _, cmd := range req.Commands {
		if cmd != nil && !cmd.New.IsZero() {
			return false
		}
	}
	return true
}

// receivePackDeletes handles a delete-only receive-pack: removes each ref
// directly from the storer, then best-effort soft-deletes the matching
// Branch row (skipped, and logged rather than failed, if it's the default
// branch — DeleteBranch refuses that the same way an ordinary
// GitManager.DeleteBranch call would; the git-level ref is still removed
// either way, matching what a real git server does when asked to delete a
// remote's HEAD branch).
func (h *smartHTTPHandler) receivePackDeletes(w http.ResponseWriter, ctx context.Context, repoName string, req *packp.ReferenceUpdateRequest) {
	repo, err := h.m.openOrInitBareRepo(ctx, repoName)
	if err != nil {
		log.Printf("[receive-pack] repo=%q: receivePackDeletes: open storer: %v", repoName, err)
		http.Error(w, "internal error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	repoRow, err := h.m.GetRepositoryByName(ctx, repoName)
	if err != nil {
		log.Printf("[receive-pack] repo=%q: receivePackDeletes: repo lookup: %v", repoName, err)
	}

	status := packp.NewReportStatus()
	status.UnpackStatus = "ok"
	for _, cmd := range req.Commands {
		if cmd == nil {
			continue
		}
		cs := &packp.CommandStatus{ReferenceName: cmd.Name, Status: "ok"}
		if rmErr := repo.Storer.RemoveReference(cmd.Name); rmErr != nil {
			cs.Status = rmErr.Error()
		} else if repoRow.ID != "" {
			h.deleteBranchRowForRef(ctx, repoRow.ID, cmd.Name.String())
			h.deleteTagRowForRef(ctx, repoRow.ID, cmd.Name.String())
		}
		status.CommandStatuses = append(status.CommandStatuses, cs)
	}

	setNoCacheHeaders(w)
	w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
	w.WriteHeader(http.StatusOK)
	_ = status.Encode(w)
}

// deleteBranchRowForRef soft-deletes the Branch row matching branchRef, if
// one exists — best-effort, see receivePackDeletes' own doc.
func (h *smartHTTPHandler) deleteBranchRowForRef(ctx context.Context, repoID, branchRef string) {
	if !strings.HasPrefix(branchRef, branchRefPrefix) {
		return // not a branch — nothing was indexed as one, see IndexPushedBranch
	}
	branchName := strings.TrimPrefix(branchRef, branchRefPrefix)
	branches, err := h.m.ListBranches(ctx, repoID)
	if err != nil {
		log.Printf("[receive-pack] repo=%s: deleteBranchRowForRef: ListBranches: %v", repoID, err)
		return
	}
	for _, b := range branches {
		if b.Name != branchName {
			continue
		}
		if err := h.m.DeleteBranch(ctx, b.ID); err != nil {
			log.Printf("[receive-pack] repo=%s: deleteBranchRowForRef: DeleteBranch %q: %v (git ref already removed)", repoID, branchName, err)
		}
		return
	}
}

// repoLoader implements gogitserver.Loader, resolving a repository name to
// its go-git storer via openOrInitBareRepo — auto-creating both the
// Repository row and its on-disk bare clone on first contact.
type repoLoader struct{ m *gitManager }

func (l *repoLoader) Load(ep *transport.Endpoint) (storer.Storer, error) {
	// ep.Path is exactly "/{repoName}" — endpointFor never encodes a rest
	// segment — so this is a plain trim, not splitRepoName (which expects
	// an HTTP *request* path, "/{repoName}/{rest}", one level deeper).
	repoName := strings.TrimPrefix(ep.Path, "/")
	if repoName == "" {
		return nil, transport.ErrRepositoryNotFound
	}
	repo, err := l.m.openOrInitBareRepo(context.Background(), repoName)
	if err != nil {
		log.Printf("[smarthttp] repo=%q: openOrInitBareRepo: %v", repoName, err)
		return nil, transport.ErrRepositoryNotFound
	}
	return repo.Storer, nil
}

// endpointFor builds the transport.Endpoint repoLoader.Load reads back —
// Path is the one field go-git's server.Loader actually uses as the
// repository key.
func endpointFor(repoName string) (*transport.Endpoint, error) {
	return transport.NewEndpoint("/" + repoName)
}

// splitRepoName splits a request path of the form "/{repoName}/rest..."
// into (repoName, "/rest...", true) — the single-tenant equivalent of the
// original's two-segment agencyID/repoName split. Strips a conventional
// ".git" suffix many git clients append to the remote name.
func splitRepoName(path string) (repoName, rest string, ok bool) {
	trimmed := strings.TrimPrefix(path, "/")
	idx := strings.Index(trimmed, "/")
	if idx < 0 {
		return "", "", false
	}
	repoName = strings.TrimSuffix(trimmed[:idx], ".git")
	rest = trimmed[idx:]
	if repoName == "" || rest == "" || rest == "/" {
		return "", "", false
	}
	return repoName, rest, true
}

func setNoCacheHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "Fri, 01 Jan 1980 00:00:00 GMT")
}

func httpErrorFromTransport(w http.ResponseWriter, err error) {
	if errors.Is(err, transport.ErrRepositoryNotFound) {
		http.Error(w, "repository not found", http.StatusNotFound)
		return
	}
	http.Error(w, "internal error: "+err.Error(), http.StatusInternalServerError)
}

// deleteTagRowForRef soft-deletes the Tag row matching tagRef, if one exists
// — best-effort, the same as deleteBranchRowForRef.
func (h *smartHTTPHandler) deleteTagRowForRef(ctx context.Context, repoID, tagRef string) {
	if !strings.HasPrefix(tagRef, tagRefPrefix) {
		return
	}
	tagName := strings.TrimPrefix(tagRef, tagRefPrefix)
	tags, err := h.m.ListTags(ctx, repoID)
	if err != nil {
		log.Printf("[receive-pack] repo=%s: deleteTagRowForRef: ListTags: %v", repoID, err)
		return
	}
	for _, t := range tags {
		if t.Name != tagName {
			continue
		}
		if err := h.m.DeleteTag(ctx, t.ID); err != nil {
			log.Printf("[receive-pack] repo=%s: deleteTagRowForRef: DeleteTag %q: %v (git ref already removed)", repoID, tagName, err)
		}
		return
	}
}
