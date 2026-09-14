package store

import (
	"database/sql"
	"errors"
)

type ActSheetInput struct {
	SheetID  int64
	Stack    int
	Rotation int64
	TX, TY   int64
}

type RegionInput struct {
	Name      string
	Kind      string
	Vertices  []Vertex
	R, G, B   int
	Tolerance int
}

func validateRegionIn(in RegionInput) error {
	if in.Name == "" {
		return errors.New("region name required")
	}
	if in.Kind != "target" && in.Kind != "blank" {
		return errors.New("region kind must be target or blank")
	}
	if len(in.Vertices) < 3 {
		return errors.New("region needs at least 3 vertices")
	}
	if in.Kind == "target" {
		if in.R < 0 || in.R > 255 || in.G < 0 || in.G > 255 || in.B < 0 || in.B > 255 {
			return errors.New("rgb must be 0..255")
		}
		if in.Tolerance < 0 || in.Tolerance > 255 {
			return errors.New("tolerance must be 0..255")
		}
	}
	return nil
}

func (s *Store) CreateAct(projectID int64, name string, sheets []ActSheetInput, regions []RegionInput) (*Act, error) {
	if name == "" {
		return nil, errors.New("act name required")
	}
	var out *Act
	err := s.tx(func(t *sql.Tx) error {
		if err := checkFrozen(t, projectID); err != nil {
			return err
		}
		var pos int
		if err := t.QueryRow(`SELECT COALESCE(MAX(position)+1,0) FROM acts WHERE project_id=?`, projectID).Scan(&pos); err != nil {
			return err
		}
		res, err := t.Exec(`INSERT INTO acts(project_id,name,position) VALUES(?,?,?)`, projectID, name, pos)
		if err != nil {
			return err
		}
		actID, _ := res.LastInsertId()
		if err := replaceActChildren(t, actID, sheets, regions); err != nil {
			return err
		}
		if err := invalidate(t, projectID); err != nil {
			return err
		}
		out, err = loadAct(t, actID)
		return err
	})
	return out, err
}

func (s *Store) ListActs(projectID int64) ([]Act, error) {
	rows, err := s.db.Query(`SELECT id,project_id,name,position FROM acts WHERE project_id=? ORDER BY position,id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	type head struct {
		id, pid int64
		name    string
		pos     int
	}
	var heads []head
	for rows.Next() {
		var h head
		if err := rows.Scan(&h.id, &h.pid, &h.name, &h.pos); err != nil {
			return nil, err
		}
		heads = append(heads, h)
		ids = append(ids, h.id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	t, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer t.Rollback()
	out := make([]Act, 0, len(heads))
	for _, h := range heads {
		a, err := loadAct(t, h.id)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, nil
}

func (s *Store) GetAct(id int64) (*Act, error) {
	t, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer t.Rollback()
	a, err := loadAct(t, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

func (s *Store) UpdateAct(id int64, name string, sheets []ActSheetInput, regions []RegionInput) (*Act, error) {
	if name == "" {
		return nil, errors.New("act name required")
	}
	var out *Act
	err := s.tx(func(t *sql.Tx) error {
		var projectID int64
		if err := t.QueryRow(`SELECT project_id FROM acts WHERE id=?`, id).Scan(&projectID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if err := checkFrozen(t, projectID); err != nil {
			return err
		}
		if _, err := t.Exec(`UPDATE acts SET name=? WHERE id=?`, name, id); err != nil {
			return err
		}
		if err := replaceActChildren(t, id, sheets, regions); err != nil {
			return err
		}
		if err := invalidate(t, projectID); err != nil {
			return err
		}
		var err error
		out, err = loadAct(t, id)
		return err
	})
	return out, err
}

func (s *Store) DeleteAct(id int64) error {
	return s.tx(func(t *sql.Tx) error {
		var projectID int64
		if err := t.QueryRow(`SELECT project_id FROM acts WHERE id=?`, id).Scan(&projectID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if err := checkFrozen(t, projectID); err != nil {
			return err
		}
		if _, err := t.Exec(`DELETE FROM acts WHERE id=?`, id); err != nil {
			return err
		}
		return invalidate(t, projectID)
	})
}

func replaceActChildren(t *sql.Tx, actID int64, sheets []ActSheetInput, regions []RegionInput) error {
	// Validate sheet references belong to the same project.
	var projectID int64
	if err := t.QueryRow(`SELECT project_id FROM acts WHERE id=?`, actID).Scan(&projectID); err != nil {
		return err
	}
	seen := map[int64]bool{}
	for _, as := range sheets {
		if seen[as.SheetID] {
			return errors.New("sheet appears twice in the same act")
		}
		seen[as.SheetID] = true
		switch as.Rotation {
		case 0, 90, 180, 270:
		default:
			return errors.New("rotation must be 0/90/180/270")
		}
		var n int
		if err := t.QueryRow(`SELECT COUNT(*) FROM sheets WHERE id=? AND project_id=?`, as.SheetID, projectID).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return errors.New("act references an unknown sheet")
		}
	}
	for i, ri := range regions {
		if err := validateRegionIn(ri); err != nil {
			return err
		}
		_ = i
	}
	if _, err := t.Exec(`DELETE FROM act_sheets WHERE act_id=?`, actID); err != nil {
		return err
	}
	for i, as := range sheets {
		// The request array order is the bottom-to-top order.
		if _, err := t.Exec(`INSERT INTO act_sheets(act_id,sheet_id,stack,rotation,tx,ty) VALUES(?,?,?,?,?,?)`,
			actID, as.SheetID, i, as.Rotation, as.TX, as.TY); err != nil {
			return err
		}
	}
	if _, err := t.Exec(`DELETE FROM regions WHERE act_id=?`, actID); err != nil {
		return err
	}
	for i, ri := range regions {
		verts, err := encodeVertices(ri.Vertices)
		if err != nil {
			return err
		}
		if _, err := t.Exec(`INSERT INTO regions(act_id,name,kind,vertices,r,g,b,tolerance,position)
			VALUES(?,?,?,?,?,?,?,?,?)`, actID, ri.Name, ri.Kind, verts, ri.R, ri.G, ri.B, ri.Tolerance, i); err != nil {
			return err
		}
	}
	return nil
}

func loadAct(t *sql.Tx, id int64) (*Act, error) {
	var a Act
	err := t.QueryRow(`SELECT id,project_id,name,position FROM acts WHERE id=?`, id).
		Scan(&a.ID, &a.ProjectID, &a.Name, &a.Position)
	if err != nil {
		return nil, err
	}
	rows, err := t.Query(`SELECT sheet_id,stack,rotation,tx,ty FROM act_sheets WHERE act_id=? ORDER BY stack,sheet_id`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var as ActSheet
		if err := rows.Scan(&as.SheetID, &as.Stack, &as.Rotation, &as.TX, &as.TY); err != nil {
			rows.Close()
			return nil, err
		}
		a.Sheets = append(a.Sheets, as)
	}
	rows.Close()
	rrows, err := t.Query(`SELECT id,act_id,name,kind,vertices,r,g,b,tolerance,position FROM regions WHERE act_id=? ORDER BY position,id`, id)
	if err != nil {
		return nil, err
	}
	for rrows.Next() {
		var rg Region
		var verts string
		if err := rrows.Scan(&rg.ID, &rg.ActID, &rg.Name, &rg.Kind, &verts, &rg.R, &rg.G, &rg.B, &rg.Tolerance, &rg.Position); err != nil {
			rrows.Close()
			return nil, err
		}
		if rg.Vertices, err = decodeVertices(verts); err != nil {
			rrows.Close()
			return nil, err
		}
		a.Regions = append(a.Regions, rg)
	}
	rrows.Close()
	return &a, nil
}
