package routes_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aosanya/mwanachama-backend-git/routes"
)

// TestG17_KeywordTreeHasNoDepthOrCountBound pins a known defect (board row
// G17, todo.md): GetKeywordTree (git_impl_keywords.go) walks a keyword's
// ENTIRE descendant subtree with no depth cap and no node-count cap,
// unlike every sibling read in this package. ListKeywords clamps its
// result to maxListPage (git_impl_keywords.go ~line 124); GetNeighborhood
// clamps traversal depth to [1,3] and node count to 100
// (git_impl_graph.go's clampDepth/neighborhoodMaxNodes). GetKeywordTree
// (git_impl_keywords.go's buildKeywordTreeNode, ~line 224) has neither —
// it recurses one DB round trip per node, unboundedly, for as many
// descendants as the caller's chosen root actually has.
//
// Driven through the real aggregator + http.ServeMux + http.Client, not
// the manager directly. Setup: a single keyword chain hangDepth levels
// deep, comfortably past maxListPage (500) so the contrast with
// ListKeywords's own cap is unambiguous.
//
// Observed: GET /keywords/tree?root=<chain root> returns every one of the
// hangDepth descendants — no truncation, no depth ceiling — while
// GET /keywords?parent_id=<some node> (a sibling read reachable through
// the same router) is bound by maxListPage. Any caller who can reach this
// route at all (there is no capability gate anywhere in this package's
// own routes/ — auth is the mounting caller's job, per this repo's own
// CLAUDE.md) can force one HTTP request to walk and serialize an
// arbitrarily large keyword taxonomy, at 2 sequential DB queries per node
// with no batching.
//
// Fix: give GetKeywordTree the same two bounds GetNeighborhood already
// has — a maximum depth and a maximum total node count, refusing or
// truncating past either. This test's "the full unbounded chain came
// back" assertion must invert into "truncated at the bound" once that
// lands.
func TestG17_KeywordTreeHasNoDepthOrCountBound(t *testing.T) {
	gm := newTestManager(t)
	mux := http.NewServeMux()
	for _, rt := range routes.Routes(gm) {
		mux.Handle(rt.Pattern(""), rt.Handler)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := srv.Client()

	const hangDepth = 600 // comfortably past maxListPage (500)

	status, root := jsonDo(t, client, http.MethodPost, srv.URL+"/keywords",
		map[string]any{"name": "g17-root"})
	if status != http.StatusCreated {
		t.Fatalf("create root keyword: status %d, body %v", status, root)
	}
	rootID := root.(map[string]any)["id"].(string)

	parentID := rootID
	for i := 0; i < hangDepth; i++ {
		status, kw := jsonDo(t, client, http.MethodPost, srv.URL+"/keywords",
			map[string]any{"name": fmt.Sprintf("g17-kw-%d", i), "parent_id": parentID})
		if status != http.StatusCreated {
			t.Fatalf("create keyword %d: status %d, body %v", i, status, kw)
		}
		parentID = kw.(map[string]any)["id"].(string)
	}

	// Control: a sibling read of the same taxonomy, through the same
	// router, is bound.
	status, listed := jsonDo(t, client, http.MethodGet,
		fmt.Sprintf("%s/keywords?parent_id=%s", srv.URL, rootID), nil)
	if status != http.StatusOK {
		t.Fatalf("ListKeywords: status %d, body %v", status, listed)
	}
	// (Only one direct child of root in this chain — this call's shape
	// isn't the interesting part; GetKeywordTree below is.)

	status, tree := jsonDo(t, client, http.MethodGet,
		fmt.Sprintf("%s/keywords/tree?root=%s", srv.URL, rootID), nil)
	if status != http.StatusOK {
		t.Fatalf("GetKeywordTree: status %d, body %v", status, tree)
	}
	got := countJSONTreeNodes(tree)
	t.Logf("GetKeywordTree(root) returned %d descendant nodes for a %d-deep chain (maxListPage=500)", got, hangDepth)

	if got < hangDepth {
		t.Fatalf("GAP not reproduced — expected the FULL unbounded chain (>= %d nodes) back with no cap, got %d; a bound may already exist", hangDepth, got)
	}
}

// countJSONTreeNodes walks a decoded GetKeywordTree response ([]any of
// {"keyword":..., "children":[...]} objects) and counts every node,
// including the root(s).
func countJSONTreeNodes(v any) int {
	nodes, ok := v.([]any)
	if !ok {
		return 0
	}
	total := 0
	for _, n := range nodes {
		total++
		if m, ok := n.(map[string]any); ok {
			total += countJSONTreeNodes(m["children"])
		}
	}
	return total
}
