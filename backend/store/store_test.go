package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func sqVerts() []Vertex {
	return []Vertex{{0, 0}, {10, 0}, {10, 10}, {0, 10}}
}

func TestProjectCRUDAndFreeze(t *testing.T) {
	st := openTestStore(t)
	p, err := st.CreateProject("p")
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != 1 || p.Frozen {
		t.Fatalf("new project wrong: %+v", p)
	}
	sh, err := st.CreateSheet(p.ID, SheetInput{Name: "s", Vertices: sqVerts(), R: 1, G: 2, B: 3, OpacityMillis: 400})
	if err != nil {
		t.Fatal(err)
	}
	act, err := st.CreateAct(p.ID, "a",
		[]ActSheetInput{{SheetID: sh.ID, Rotation: 0, TX: 0, TY: 0}},
		[]RegionInput{{Name: "r", Kind: "blank", Vertices: sqVerts()}})
	if err != nil {
		t.Fatal(err)
	}
	// An edit bumps the version.
	before, _ := st.GetProject(p.ID)
	if _, err := st.UpdateSheet(sh.ID, SheetInput{Name: "s2", Vertices: sqVerts(), R: 1, G: 2, B: 3, OpacityMillis: 401}); err != nil {
		t.Fatal(err)
	}
	p2, _ := st.GetProject(p.ID)
	if p2.Version != before.Version+1 {
		t.Fatalf("version after edit: %d -> %d", before.Version, p2.Version)
	}

	// Simulate a passing report and confirmation.
	rep, err := st.CreateReportVersioned(p2.Version, p.ID, "passed", "{}", "{}")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ConfirmReport(rep.ID); err != nil {
		t.Fatal(err)
	}
	// Editing is now rejected.
	if _, err := st.UpdateSheet(sh.ID, SheetInput{Name: "s3", Vertices: sqVerts(), R: 1, G: 2, B: 3, OpacityMillis: 402}); !errors.Is(err, ErrFrozen) {
		t.Fatalf("want ErrFrozen, got %v", err)
	}
	if err := st.DeleteAct(act.ID); !errors.Is(err, ErrFrozen) {
		t.Fatalf("want ErrFrozen, got %v", err)
	}
	// Unfreeze restores edits and expires the report.
	if err := st.UnfreezeProject(p.ID); err != nil {
		t.Fatal(err)
	}
	r2, _ := st.GetReport(rep.ID)
	if !r2.Expired {
		t.Fatal("report should be expired after unfreeze")
	}
}

func TestCannotConfirmFailedOrExpired(t *testing.T) {
	st := openTestStore(t)
	p, _ := st.CreateProject("p")
	cur := func() int64 {
		pp, err := st.GetProject(p.ID)
		if err != nil {
			t.Fatal(err)
		}
		return pp.Version
	}
	fail, err := st.CreateReportVersioned(cur(), p.ID, "failed", "{}", "{}")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ConfirmReport(fail.ID); err == nil {
		t.Fatal("failed report must not confirm")
	}
	pass, _ := st.CreateReportVersioned(cur(), p.ID, "passed", "{}", "{}")
	// An edit expires the passing report and bumps the version.
	st.CreateSheet(p.ID, SheetInput{Name: "x", Vertices: sqVerts(), R: 0, G: 0, B: 0, OpacityMillis: 1000})
	if _, err := st.ConfirmReport(pass.ID); err == nil {
		t.Fatal("expired report must not confirm")
	}
}

// A report whose snapshot version no longer matches must never be stored:
// simulate an edit racing the proof by attempting insert against an old
// version.
func TestVersionedReportRejectsRacedEdit(t *testing.T) {
	st := openTestStore(t)
	p, _ := st.CreateProject("p")
	st.CreateSheet(p.ID, SheetInput{Name: "x", Vertices: sqVerts(), R: 0, G: 0, B: 0, OpacityMillis: 1000})
	p2, _ := st.GetProject(p.ID)
	// Pretend the proof ran against version 1 but inputs are now at version 2.
	if _, err := st.CreateReportVersioned(p2.Version-1, p.ID, "passed", "{}", "{}"); !errors.Is(err, ErrStaleReport) {
		t.Fatalf("want ErrStaleReport, got %v", err)
	}
	// Current version is accepted.
	if _, err := st.CreateReportVersioned(p2.Version, p.ID, "passed", "{}", "{}"); err != nil {
		t.Fatalf("current version must be accepted: %v", err)
	}
}

// Renaming a frozen project is rejected; renaming a live one invalidates
// existing reports because the name is part of the frozen input snapshot.
func TestRenameFreezeAndInvalidation(t *testing.T) {
	st := openTestStore(t)
	p, _ := st.CreateProject("before")
	rep, err := st.CreateReportVersioned(p.Version, p.ID, "passed", "{}", "{}")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RenameProject(p.ID, "after"); err != nil {
		t.Fatal(err)
	}
	r2, _ := st.GetReport(rep.ID)
	if !r2.Expired {
		t.Fatal("rename must expire existing reports")
	}
	pp, _ := st.GetProject(p.ID)
	pass2, _ := st.CreateReportVersioned(pp.Version, p.ID, "passed", "{}", "{}")
	if _, err := st.ConfirmReport(pass2.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.RenameProject(p.ID, "third"); !errors.Is(err, ErrFrozen) {
		t.Fatalf("rename while frozen must fail, got %v", err)
	}
	got, _ := st.GetProject(p.ID)
	if got.Name != "after" {
		t.Fatalf("frozen rename changed name: %s", got.Name)
	}
	if err := st.DeleteProject(p.ID); !errors.Is(err, ErrFrozen) {
		t.Fatalf("delete while frozen must fail, got %v", err)
	}
}

func TestRejectsBadRotationAndUnknownSheet(t *testing.T) {
	st := openTestStore(t)
	p, _ := st.CreateProject("p")
	sh, _ := st.CreateSheet(p.ID, SheetInput{Name: "s", Vertices: sqVerts(), R: 0, G: 0, B: 0, OpacityMillis: 1000})
	if _, err := st.CreateAct(p.ID, "a",
		[]ActSheetInput{{SheetID: sh.ID, Rotation: 45}}, nil); err == nil {
		t.Fatal("rotation 45 must be rejected")
	}
	if _, err := st.CreateAct(p.ID, "a",
		[]ActSheetInput{{SheetID: 99999, Rotation: 0}}, nil); err == nil {
		t.Fatal("unknown sheet must be rejected")
	}
	if _, err := st.CreateAct(p.ID, "a",
		[]ActSheetInput{{SheetID: sh.ID, Rotation: 0}},
		[]RegionInput{{Name: "r", Kind: "weird", Vertices: sqVerts()}}); err == nil {
		t.Fatal("bad region kind must be rejected")
	}
}

func TestNotFound(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.GetProject(404); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := st.DeleteProject(404); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
