package routes_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const conflictBaseContent = "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\n"

func conflictContent(line2 string) string {
	return strings.Replace(conflictBaseContent, "line2", line2, 1)
}

func decodeID(t *testing.T, body []byte) string {
	t.Helper()
	var row map[string]any
	if err := json.Unmarshal(body, &row); err != nil {
		t.Fatalf("decode: %v: %s", err, body)
	}
	id, _ := row["id"].(string)
	if id == "" {
		t.Fatalf("no id in %s", body)
	}
	return id
}

func TestCompletingAConflictingMergeRequestAnswersConflictAndFailsTheRequest(t *testing.T) {
	srv := serve(t)

	code, body := call(t, srv, "POST", "/repos", `{"name":"widgets"}`)
	if code != http.StatusCreated {
		t.Fatalf("create repository: %d %s", code, body)
	}
	repoID := decodeID(t, body)

	code, body = call(t, srv, "GET", "/repos/"+repoID+"/branches", "")
	var branches []map[string]any
	if err := json.Unmarshal(body, &branches); err != nil || code != http.StatusOK || len(branches) == 0 {
		t.Fatalf("list branches: %d %s", code, body)
	}
	defaultID, _ := branches[0]["id"].(string)

	write := func(branchID, content string) {
		t.Helper()
		payload, _ := json.Marshal(map[string]string{
			"path": "notes.txt", "content": content, "message": "edit", "author_name": "amos",
		})
		if code, body := call(t, srv, "POST", "/branches/"+branchID+"/files", string(payload)); code != http.StatusCreated {
			t.Fatalf("write file: %d %s", code, body)
		}
	}

	write(defaultID, conflictBaseContent)
	code, body = call(t, srv, "POST", "/repos/"+repoID+"/branches",
		`{"name":"feature/a","from_branch_id":"`+defaultID+`"}`)
	if code != http.StatusCreated {
		t.Fatalf("create branch: %d %s", code, body)
	}
	featureID := decodeID(t, body)
	write(featureID, conflictContent("from branch"))
	write(defaultID, conflictContent("from default"))

	code, body = call(t, srv, "POST", "/merge-requests",
		`{"repository_id":"`+repoID+`","title":"conflicting change","source_branch_id":"`+featureID+`"}`)
	if code != http.StatusCreated {
		t.Fatalf("open merge request: %d %s", code, body)
	}
	mrID := decodeID(t, body)

	code, body = call(t, srv, "POST", "/merge-requests/"+mrID+"/complete", `{}`)
	if code != http.StatusConflict {
		t.Fatalf("completing a conflicting merge request must be 409, got %d: %s", code, body)
	}
	if !strings.Contains(string(body), "notes.txt") {
		t.Errorf("the 409 must name the conflicting file, got %s", body)
	}

	code, body = call(t, srv, "GET", "/merge-requests/"+mrID, "")
	if code != http.StatusOK {
		t.Fatalf("read merge request: %d %s", code, body)
	}
	var mr map[string]any
	if err := json.Unmarshal(body, &mr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if mr["status"] != "failed" {
		t.Errorf("status = %v, want failed", mr["status"])
	}
	if msg, _ := mr["error_message"].(string); !strings.Contains(msg, "notes.txt") {
		t.Errorf("error_message must keep the conflict text naming notes.txt, got %q", msg)
	}
}
