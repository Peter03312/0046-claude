package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"storyproof/store"
)

func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db") + "?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return NewServer(st), dir
}

func do(t *testing.T, h http.Handler, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("non-json response %d: %s", rec.Code, rec.Body.String())
		}
	}
	return rec.Code, out
}

func mustInt(t *testing.T, v any, what string) int64 {
	t.Helper()
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("%s is not a number: %#v", what, v)
	}
	return int64(f)
}

func sq(x0, y0, x1, y1 int64) []pointDTO {
	return []pointDTO{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
}

func createProject(t *testing.T, h http.Handler, name string) int64 {
	code, body := do(t, h, "POST", "/api/projects", map[string]any{"name": name})
	if code != 201 {
		t.Fatalf("create project: %d %v", code, body)
	}
	return mustInt(t, body["id"], "project id")
}

func createSheet(t *testing.T, h http.Handler, pid int64, body map[string]any) int64 {
	code, resp := do(t, h, "POST", "/api/projects/"+itoaN(pid)+"/sheets", body)
	if code != 201 {
		t.Fatalf("create sheet: %d %v", code, resp)
	}
	return mustInt(t, resp["id"], "sheet id")
}

// TestFullProofLifecycle: passing proof, edit invalidates, frozen edits fail.
func TestFullProofLifecycle(t *testing.T) {
	s, _ := newTestServer(t)
	h := s.Handler()
	pid := createProject(t, h, "night sky")

	redBody := map[string]any{
		"name": "red pane", "vertices": sq(0, 0, 10, 10),
		"r": 200, "g": 0, "b": 0, "opacityMillis": 1000,
	}
	sid := createSheet(t, h, pid, redBody)

	actBody := map[string]any{
		"name": "act 1",
		"sheets": []map[string]any{
			{"sheetId": sid, "rotation": 0, "tx": 0, "ty": 0},
		},
		"regions": []map[string]any{
			{"name": "frame", "kind": "target", "vertices": sq(0, 0, 10, 10),
				"r": 200, "g": 0, "b": 0, "tolerance": 0},
		},
	}
	code, actResp := do(t, h, "POST", "/api/projects/"+itoaN(pid)+"/acts", actBody)
	if code != 201 {
		t.Fatalf("create act: %d %v", code, actResp)
	}
	aid := mustInt(t, actResp["id"], "act id")

	// First proof passes.
	code, rep := do(t, h, "POST", "/api/projects/"+itoaN(pid)+"/proofs", nil)
	if code != 201 {
		t.Fatalf("proof run: %d %v", code, rep)
	}
	if rep["status"] != "passed" {
		t.Fatalf("expected passed, got %v", rep["status"])
	}
	rid := mustInt(t, rep["id"], "report id")

	// Confirm freezes inputs.
	code, conf := do(t, h, "POST", "/api/reports/"+itoaN(rid)+"/confirm", nil)
	if code != 200 {
		t.Fatalf("confirm: %d %v", code, conf)
	}
	if conf["status"] != "passed" {
		t.Fatalf("confirm payload: %v", conf)
	}

	// Any edit is rejected while frozen.
	code, blocked := do(t, h, "PUT", "/api/sheets/"+itoaN(sid), redBody)
	if code != http.StatusConflict {
		t.Fatalf("frozen edit status=%d body=%v", code, blocked)
	}
	code, blocked2 := do(t, h, "DELETE", "/api/acts/"+itoaN(aid), nil)
	if code != http.StatusConflict {
		t.Fatalf("frozen act delete status=%d body=%v", code, blocked2)
	}

	// Unfreeze, then edit: old report is expired (410).
	code, _ = do(t, h, "POST", "/api/projects/"+itoaN(pid)+"/unfreeze", nil)
	if code != 200 {
		t.Fatalf("unfreeze: %d", code)
	}
	code, got := do(t, h, "GET", "/api/reports/"+itoaN(rid), nil)
	if code != http.StatusGone {
		t.Fatalf("expected 410 expired, got %d", code)
	}
	if got["expired"] != true {
		t.Fatalf("expected expired=true: %v", got)
	}
}

// TestFailedProofHasBadCells: a 1-unit sliver leaks into the blank region and
// the failed report cannot be confirmed.
func TestFailedProofHasBadCells(t *testing.T) {
	s, _ := newTestServer(t)
	h := s.Handler()
	pid := createProject(t, h, "slivers")
	sid := createSheet(t, h, pid, map[string]any{
		"name": "pane", "vertices": sq(0, 0, 11, 10),
		"r": 0, "g": 0, "b": 0, "opacityMillis": 1000,
	})
	do(t, h, "POST", "/api/projects/"+itoaN(pid)+"/acts", map[string]any{
		"name": "act",
		"sheets": []map[string]any{
			{"sheetId": sid, "rotation": 0, "tx": 0, "ty": 0},
		},
		"regions": []map[string]any{
			{"name": "sky", "kind": "blank", "vertices": sq(10, 0, 20, 10)},
		},
	})
	code, rep := do(t, h, "POST", "/api/projects/"+itoaN(pid)+"/proofs", nil)
	if code != 201 {
		t.Fatalf("proof: %d %v", code, rep)
	}
	if rep["status"] != "failed" {
		t.Fatalf("expected failed: %v", rep["status"])
	}
	result := rep["result"].(map[string]any)
	if result["failedAct"].(float64) != 0 {
		t.Fatalf("failedAct: %v", result["failedAct"])
	}
	acts := result["acts"].([]any)
	ar := acts[0].(map[string]any)
	cells := ar["badCells"].([]any)
	if len(cells) != 1 {
		t.Fatalf("want 1 bad cell, got %d", len(cells))
	}
	cell := cells[0].(map[string]any)
	if cell["kind"] != "leak" {
		t.Fatalf("kind: %v", cell["kind"])
	}
	outer := cell["outer"].([]any)
	if len(outer) != 4 {
		t.Fatalf("outer ring vertices: %d", len(outer))
	}
	contribs := cell["contributors"].([]any)
	if len(contribs) != 1 {
		t.Fatalf("contributors: %v", contribs)
	}

	rid := mustInt(t, rep["id"], "rid")
	code, body := do(t, h, "POST", "/api/reports/"+itoaN(rid)+"/confirm", nil)
	if code != http.StatusConflict {
		t.Fatalf("confirming failed report: %d %v", code, body)
	}
}

// TestOnlyEarliestFailingActReturned: the second act also fails but the proof
// stops at act one and returns only its cells.
func TestOnlyEarliestFailingActReturned(t *testing.T) {
	s, _ := newTestServer(t)
	h := s.Handler()
	pid := createProject(t, h, "two acts")
	sid := createSheet(t, h, pid, map[string]any{
		"name": "pane", "vertices": sq(0, 0, 11, 10),
		"r": 0, "g": 0, "b": 0, "opacityMillis": 1000,
	})
	actSheet := []map[string]any{{"sheetId": sid, "rotation": 0, "tx": 0, "ty": 0}}
	blank := []map[string]any{{"name": "sky", "kind": "blank", "vertices": sq(10, 0, 20, 10)}}
	for _, n := range []string{"one", "two"} {
		code, resp := do(t, h, "POST", "/api/projects/"+itoaN(pid)+"/acts",
			map[string]any{"name": n, "sheets": actSheet, "regions": blank})
		if code != 201 {
			t.Fatalf("act %s: %d %v", n, code, resp)
		}
	}
	_, rep := do(t, h, "POST", "/api/projects/"+itoaN(pid)+"/proofs", nil)
	result := rep["result"].(map[string]any)
	acts := result["acts"].([]any)
	if len(acts) != 1 {
		t.Fatalf("proof must stop at earliest failing act, got %d", len(acts))
	}
	if acts[0].(map[string]any)["name"] != "one" {
		t.Fatalf("earliest act: %v", acts[0])
	}
}

// TestSelfIntersectingInputRejected: bowtie sheet => 422, no freeze possible.
func TestSelfIntersectingInputRejected(t *testing.T) {
	s, _ := newTestServer(t)
	h := s.Handler()
	pid := createProject(t, h, "bad")
	sid := createSheet(t, h, pid, map[string]any{
		"name":     "bowtie",
		"vertices": []pointDTO{{0, 0}, {10, 10}, {10, 0}, {0, 10}},
		"r":        1, "g": 2, "b": 3, "opacityMillis": 500,
	})
	do(t, h, "POST", "/api/projects/"+itoaN(pid)+"/acts", map[string]any{
		"name": "a",
		"sheets": []map[string]any{
			{"sheetId": sid, "rotation": 0, "tx": 0, "ty": 0},
		},
		"regions": []map[string]any{
			{"name": "sky", "kind": "blank", "vertices": sq(-20, -20, -10, -10)},
		},
	})
	code, rep := do(t, h, "POST", "/api/projects/"+itoaN(pid)+"/proofs", nil)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %v", code, rep)
	}
	result := rep["result"].(map[string]any)
	ar := result["acts"].([]any)[0].(map[string]any)
	if ar["status"] != "invalid" || ar["error"] == "" {
		t.Fatalf("invalid act payload: %v", ar)
	}
}

// TestEditingExpiresReports: run a passing proof, edit one vertex, old report
// expires and version increments; reports list shows it expired.
func TestEditingExpiresReports(t *testing.T) {
	s, _ := newTestServer(t)
	h := s.Handler()
	pid := createProject(t, h, "v")
	body := map[string]any{
		"name": "p", "vertices": sq(0, 0, 10, 10),
		"r": 0, "g": 0, "b": 0, "opacityMillis": 1000,
	}
	sid := createSheet(t, h, pid, body)
	do(t, h, "POST", "/api/projects/"+itoaN(pid)+"/acts", map[string]any{
		"name":   "a",
		"sheets": []map[string]any{{"sheetId": sid, "rotation": 0, "tx": 0, "ty": 0}},
		"regions": []map[string]any{
			{"name": "t", "kind": "target", "vertices": sq(0, 0, 10, 10),
				"r": 0, "g": 0, "b": 0, "tolerance": 0},
		},
	})
	_, rep := do(t, h, "POST", "/api/projects/"+itoaN(pid)+"/proofs", nil)
	rid := mustInt(t, rep["id"], "rid")
	ver := rep["version"].(float64)

	body["opacityMillis"] = 250
	if code, r2 := do(t, h, "PUT", "/api/sheets/"+itoaN(sid), body); code != 200 {
		t.Fatalf("edit sheet: %d %v", code, r2)
	}
	code, got := do(t, h, "GET", "/api/reports/"+itoaN(rid), nil)
	if code != http.StatusGone {
		t.Fatalf("expected gone, %d", code)
	}
	if got["version"].(float64) != ver {
		t.Fatalf("old report version changed")
	}
	code, raw := doRaw(t, h, "GET", "/api/projects/"+itoaN(pid)+"/reports")
	if code != 200 {
		t.Fatalf("list reports: %d", code)
	}
	var list []map[string]any
	if err := json.Unmarshal(raw, &list); err != nil || len(list) != 1 || list[0]["expired"] != true {
		t.Fatalf("reports list wrong: %d %s", code, string(raw))
	}
}

func doRaw(t *testing.T, h http.Handler, method, path string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

// TestRotationProof: 90-degree rotation around origin places the pane over a
// target located in the rotated quadrant, and keeps the original quadrant blank.
func TestRotationProof(t *testing.T) {
	s, _ := newTestServer(t)
	h := s.Handler()
	pid := createProject(t, h, "rotate")
	sid := createSheet(t, h, pid, map[string]any{
		"name": "p", "vertices": sq(0, 0, 10, 10),
		"r": 33, "g": 44, "b": 55, "opacityMillis": 1000,
	})
	code, resp := do(t, h, "POST", "/api/projects/"+itoaN(pid)+"/acts", map[string]any{
		"name": "a",
		"sheets": []map[string]any{
			{"sheetId": sid, "rotation": 90, "tx": 0, "ty": 0},
		},
		"regions": []map[string]any{
			{"name": "rotated target", "kind": "target", "vertices": sq(-10, 0, 0, 10),
				"r": 33, "g": 44, "b": 55, "tolerance": 0},
			{"name": "original spot blank", "kind": "blank", "vertices": sq(0, 0, 10, 10)},
		},
	})
	if code != 201 {
		t.Fatalf("act: %d %v", code, resp)
	}
	code, rep := do(t, h, "POST", "/api/projects/"+itoaN(pid)+"/proofs", nil)
	if code != 201 || rep["status"] != "passed" {
		t.Fatalf("rotated proof: %d %v", code, rep)
	}
}

// TestValidationErrors: bad colour / opacity / enum.
func TestValidationErrors(t *testing.T) {
	s, _ := newTestServer(t)
	h := s.Handler()
	pid := createProject(t, h, "v")
	code, body := do(t, h, "POST", "/api/projects/"+itoaN(pid)+"/sheets", map[string]any{
		"name": "x", "vertices": sq(0, 0, 4, 4),
		"r": 300, "g": 0, "b": 0, "opacityMillis": 500,
	})
	if code != 400 {
		t.Fatalf("bad r: %d", code)
	}
	if body["error"] == "" {
		t.Fatal("error message")
	}
	code, _ = do(t, h, "POST", "/api/projects/"+itoaN(pid)+"/sheets", map[string]any{
		"name": "x", "vertices": sq(0, 0, 4, 4),
		"r": 0, "g": 0, "b": 0, "opacityMillis": 1001,
	})
	if code != 400 {
		t.Fatalf("bad opacity: %d", code)
	}
	code, _ = do(t, h, "POST", "/api/projects", map[string]any{"name": 123})
	if code != 400 {
		t.Fatalf("bad name type: %d", code)
	}
}

func itoaN(v int64) string { return strconv.FormatInt(v, 10) }

// An edit landing while the proof is being computed must never be labelled
// with the pre-edit verdict: the whole run is rejected (409) and no report is
// recorded. The hook deterministically performs the edit after evaluation but
// before the version-checked insert.
func TestProofEditDuringRunRejected(t *testing.T) {
	s, _ := newTestServer(t)
	h := s.Handler()
	pid := createProject(t, h, "racing")
	sid := createSheet(t, h, pid, map[string]any{
		"name": "p", "vertices": sq(0, 0, 4, 4),
		"r": 0, "g": 0, "b": 0, "opacityMillis": 1000,
	})
	do(t, h, "POST", "/api/projects/"+itoaN(pid)+"/acts", map[string]any{
		"name": "a",
		"sheets": []map[string]any{
			{"sheetId": sid, "rotation": 0, "tx": 0, "ty": 0},
		},
		"regions": []map[string]any{
			{"name": "t", "kind": "target", "vertices": sq(0, 0, 4, 4),
				"r": 0, "g": 0, "b": 0, "tolerance": 0},
		},
	})
	s.beforePersistProof = func(projectID int64) error {
		// Simulate the parent editing the manuscript mid-proof.
		if err := s.st.RenameProject(projectID, "edited mid-run"); err != nil {
			return err
		}
		return nil
	}
	code, body := do(t, h, "POST", "/api/projects/"+itoaN(pid)+"/proofs", nil)
	if code != http.StatusConflict {
		t.Fatalf("raced proof status=%d body=%v", code, body)
	}
	// No report from the stale run.
	code, raw := doRaw(t, h, "GET", "/api/projects/"+itoaN(pid)+"/reports")
	if code != 200 {
		t.Fatalf("list reports: %d", code)
	}
	var reps []map[string]any
	if err := json.Unmarshal(raw, &reps); err != nil || len(reps) != 0 {
		t.Fatalf("stale run must not be recorded, got %s", string(raw))
	}
}

// Renaming a frozen project is blocked; renaming a live one succeeds and
// expires existing reports.
func TestRenameFreezeSemantics(t *testing.T) {
	s, _ := newTestServer(t)
	h := s.Handler()
	pid := createProject(t, h, "orig")
	code, body := do(t, h, "PATCH", "/api/projects/"+itoaN(pid), map[string]any{"name": "live-renamed"})
	if code != 200 {
		t.Fatalf("live rename: %d %v", code, body)
	}
	sid := createSheet(t, h, pid, map[string]any{
		"name": "p", "vertices": sq(0, 0, 4, 4),
		"r": 0, "g": 0, "b": 0, "opacityMillis": 1000,
	})
	do(t, h, "POST", "/api/projects/"+itoaN(pid)+"/acts", map[string]any{
		"name":   "a",
		"sheets": []map[string]any{{"sheetId": sid, "rotation": 0, "tx": 0, "ty": 0}},
		"regions": []map[string]any{
			{"name": "t", "kind": "target", "vertices": sq(0, 0, 4, 4),
				"r": 0, "g": 0, "b": 0, "tolerance": 0},
		},
	})
	rep := mustReport(t, h, pid)
	if rep["status"] != "passed" {
		t.Fatalf("want passed: %v", rep["status"])
	}
	rid := mustInt(t, rep["id"], "rid")
	code, conf := do(t, h, "POST", "/api/reports/"+itoaN(rid)+"/confirm", nil)
	if code != 200 {
		t.Fatalf("confirm: %d %v", code, conf)
	}
	code, blocked := do(t, h, "PATCH", "/api/projects/"+itoaN(pid), map[string]any{"name": "sneaky"})
	if code != http.StatusConflict {
		t.Fatalf("rename frozen project status=%d body=%v", code, blocked)
	}
	got := doGetProject(t, h, pid)
	if got["name"] != "live-renamed" {
		t.Fatalf("frozen rename leaked: %v", got["name"])
	}
}

func mustReport(t *testing.T, h http.Handler, pid int64) map[string]any {
	t.Helper()
	code, rep := do(t, h, "POST", "/api/projects/"+itoaN(pid)+"/proofs", nil)
	if code != 201 {
		t.Fatalf("proof: %d %v", code, rep)
	}
	return rep
}

func doGetProject(t *testing.T, h http.Handler, pid int64) map[string]any {
	t.Helper()
	code, body := do(t, h, "GET", "/api/projects/"+itoaN(pid), nil)
	if code != 200 {
		t.Fatalf("get project: %d", code)
	}
	return body["project"].(map[string]any)
}
