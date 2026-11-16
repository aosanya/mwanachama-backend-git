package mwanachamagit

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing/format/pktline"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gogitserver "github.com/go-git/go-git/v5/plumbing/transport/server"
)

const (
	refusedDefaultBranch = "push to a merge request, not to the default branch"
	refusedNotABranch    = "only branches can be pushed to the hub"
	refusedWithOthers    = "not applied because another ref in this push was refused"
	maxDrainedPushBytes  = 64 << 20
)

type snapshotLoader struct{ storage storer.Storer }

func (l snapshotLoader) Load(*transport.Endpoint) (storer.Storer, error) { return l.storage, nil }

type fileSmartHTTP struct{ m *gitManager }

func (r *RepoFile) GitHTTPHandler() http.Handler {
	return &fileSmartHTTP{m: r.GitManager.(*gitManager)}
}

func (h *fileSmartHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/info/refs":
		h.infoRefs(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/git-upload-pack":
		h.uploadPack(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/git-receive-pack":
		h.receivePack(w, r)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (h *fileSmartHTTP) session(w http.ResponseWriter, r *http.Request) (*fileSnapshot, transport.Transport, *transport.Endpoint, bool) {
	return h.openSession(w, r, h.m.snapshotFile)
}

func (h *fileSmartHTTP) refsSession(w http.ResponseWriter, r *http.Request) (*fileSnapshot, transport.Transport, *transport.Endpoint, bool) {
	return h.openSession(w, r, h.m.snapshotRefs)
}

func (h *fileSmartHTTP) openSession(w http.ResponseWriter, r *http.Request, take func(context.Context) (*fileSnapshot, error)) (*fileSnapshot, transport.Transport, *transport.Endpoint, bool) {
	snap, err := take(r.Context())
	if err != nil {
		http.Error(w, "repository not found", http.StatusNotFound)
		return nil, nil, nil, false
	}
	ep, err := transport.NewEndpoint("/repository")
	if err != nil {
		http.Error(w, "bad endpoint", http.StatusInternalServerError)
		return nil, nil, nil, false
	}
	return snap, gogitserver.NewServer(snapshotLoader{storage: snap.storage}), ep, true
}

func (h *fileSmartHTTP) infoRefs(w http.ResponseWriter, r *http.Request) {
	service := r.URL.Query().Get("service")
	if service != transport.UploadPackServiceName && service != transport.ReceivePackServiceName {
		http.Error(w, "unsupported service", http.StatusForbidden)
		return
	}
	_, srv, ep, ok := h.refsSession(w, r)
	if !ok {
		return
	}
	var advRefs *packp.AdvRefs
	var err error
	if service == transport.UploadPackServiceName {
		var sess transport.UploadPackSession
		if sess, err = srv.NewUploadPackSession(ep, nil); err == nil {
			defer sess.Close() //nolint:errcheck
			advRefs, err = sess.AdvertisedReferencesContext(r.Context())
		}
	} else {
		var sess transport.ReceivePackSession
		if sess, err = srv.NewReceivePackSession(ep, nil); err == nil {
			defer sess.Close() //nolint:errcheck
			advRefs, err = sess.AdvertisedReferencesContext(r.Context())
		}
	}
	if err != nil {
		httpErrorFromTransport(w, err)
		return
	}
	advRefs.Prefix = [][]byte{[]byte(fmt.Sprintf("# service=%s", service)), pktline.Flush}
	setNoCacheHeaders(w)
	w.Header().Set("Content-Type", fmt.Sprintf("application/x-git-%s-advertisement", strings.TrimPrefix(service, "git-")))
	w.WriteHeader(http.StatusOK)
	_ = advRefs.Encode(w)
}

func (h *fileSmartHTTP) uploadPack(w http.ResponseWriter, r *http.Request) {
	_, srv, ep, ok := h.session(w, r)
	if !ok {
		return
	}
	sess, err := srv.NewUploadPackSession(ep, nil)
	if err != nil {
		httpErrorFromTransport(w, err)
		return
	}
	defer sess.Close() //nolint:errcheck
	if _, err := sess.AdvertisedReferencesContext(r.Context()); err != nil {
		httpErrorFromTransport(w, err)
		return
	}
	req := packp.NewUploadPackRequest()
	if err := req.Decode(r.Body); err != nil {
		http.Error(w, "malformed upload-pack request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !req.Depth.IsZero() || len(req.Shallows) > 0 {
		http.Error(w, "shallow requests are not supported", http.StatusNotImplemented)
		return
	}
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

func (h *fileSmartHTTP) receivePack(w http.ResponseWriter, r *http.Request) {
	snap, srv, ep, ok := h.session(w, r)
	if !ok {
		return
	}
	sess, err := srv.NewReceivePackSession(ep, nil)
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

	if refusals := pushRefusals(snap.repo.DefaultBranch, req); len(refusals) > 0 {
		_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, maxDrainedPushBytes))
		writeReceiveStatus(w, refusedStatus(req, refusals))
		return
	}

	if isDeleteOnly(req) {
		writeReceiveStatus(w, h.deleteBranches(r.Context(), snap, req))
		return
	}

	status, err := sess.ReceivePack(r.Context(), req)
	if err != nil {
		log.Printf("[git-file] receive-pack: %v", err)
		httpErrorFromTransport(w, err)
		return
	}
	h.indexAccepted(r.Context(), snap, req, status)
	writeReceiveStatus(w, status)
}

func pushRefusals(defaultBranch string, req *packp.ReferenceUpdateRequest) map[string]string {
	refusals := map[string]string{}
	defaultRef := branchRefPrefix + defaultBranch
	for _, cmd := range req.Commands {
		name := cmd.Name.String()
		switch {
		case name == defaultRef:
			refusals[name] = refusedDefaultBranch
		case !strings.HasPrefix(name, branchRefPrefix):
			refusals[name] = refusedNotABranch
		}
	}
	return refusals
}

func refusedStatus(req *packp.ReferenceUpdateRequest, refusals map[string]string) *packp.ReportStatus {
	status := packp.NewReportStatus()
	status.UnpackStatus = "ok"
	for _, cmd := range req.Commands {
		reason, refused := refusals[cmd.Name.String()]
		if !refused {
			reason = refusedWithOthers
		}
		status.CommandStatuses = append(status.CommandStatuses, &packp.CommandStatus{ReferenceName: cmd.Name, Status: reason})
	}
	return status
}

func (h *fileSmartHTTP) deleteBranches(ctx context.Context, snap *fileSnapshot, req *packp.ReferenceUpdateRequest) *packp.ReportStatus {
	status := packp.NewReportStatus()
	status.UnpackStatus = "ok"
	for _, cmd := range req.Commands {
		cs := &packp.CommandStatus{ReferenceName: cmd.Name, Status: "ok"}
		if err := h.deleteBranchRow(ctx, snap.repo.ID, strings.TrimPrefix(cmd.Name.String(), branchRefPrefix)); err != nil {
			cs.Status = err.Error()
		}
		status.CommandStatuses = append(status.CommandStatuses, cs)
	}
	return status
}

func (h *fileSmartHTTP) deleteBranchRow(ctx context.Context, repoID, name string) error {
	branches, err := h.m.ListBranches(ctx, repoID)
	if err != nil {
		return err
	}
	for _, b := range branches {
		if b.Name == name {
			return h.m.DeleteBranch(ctx, b.ID)
		}
	}
	return nil
}

func (h *fileSmartHTTP) indexAccepted(ctx context.Context, snap *fileSnapshot, req *packp.ReferenceUpdateRequest, status *packp.ReportStatus) {
	repo, err := snap.repository()
	if err != nil {
		failEveryCommand(status, err)
		return
	}
	failed := map[string]error{}
	for _, cmd := range req.Commands {
		if cmd.New.IsZero() {
			if err := h.deleteBranchRow(ctx, snap.repo.ID, strings.TrimPrefix(cmd.Name.String(), branchRefPrefix)); err != nil {
				failed[cmd.Name.String()] = err
			}
			continue
		}
		err := h.m.indexPushedBranchFrom(ctx, repo, snap.repo, cmd.Name.String(), cmd.Old.String(), cmd.New.String(), time.Now())
		if err == nil {
			err = h.m.storePushedObjects(ctx, repo, snap.repo, cmd.New)
		}
		if err != nil {
			failed[cmd.Name.String()] = err
		}
	}
	for _, cs := range status.CommandStatuses {
		if err, bad := failed[cs.ReferenceName.String()]; bad {
			log.Printf("[git-file] ref=%s: %v", cs.ReferenceName, err)
			cs.Status = "the hub could not record this push: " + err.Error()
		}
	}
}

func failEveryCommand(status *packp.ReportStatus, err error) {
	for _, cs := range status.CommandStatuses {
		cs.Status = "the hub could not record this push: " + err.Error()
	}
}

func writeReceiveStatus(w http.ResponseWriter, status *packp.ReportStatus) {
	var body bytes.Buffer
	_ = status.Encode(&body)
	setNoCacheHeaders(w)
	w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body.Bytes())
}
