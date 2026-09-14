package store

import (
	"database/sql"
	"errors"
)

type SheetInput struct {
	Name          string
	Vertices      []Vertex
	R, G, B       int
	OpacityMillis int
}

func validateSheetIn(in SheetInput) error {
	if in.Name == "" {
		return errors.New("sheet name required")
	}
	if len(in.Vertices) < 3 {
		return errors.New("sheet needs at least 3 vertices")
	}
	if in.R < 0 || in.R > 255 || in.G < 0 || in.G > 255 || in.B < 0 || in.B > 255 {
		return errors.New("rgb must be 0..255")
	}
	if in.OpacityMillis < 0 || in.OpacityMillis > 1000 {
		return errors.New("opacityMillis must be 0..1000")
	}
	return nil
}

func scanSheet(rows interface {
	Scan(...any) error
}) (*Sheet, error) {
	var sh Sheet
	var verts string
	var pid int64
	if err := rows.Scan(&sh.ID, &pid, &sh.Name, &verts, &sh.R, &sh.G, &sh.B, &sh.OpacityMillis, &sh.Position); err != nil {
		return nil, err
	}
	sh.ProjectID = pid
	var err error
	if sh.Vertices, err = decodeVertices(verts); err != nil {
		return nil, err
	}
	return &sh, nil
}

const sheetCols = `id, project_id, name, vertices, r, g, b, opacity, position`

func (s *Store) CreateSheet(projectID int64, in SheetInput) (*Sheet, error) {
	if err := validateSheetIn(in); err != nil {
		return nil, err
	}
	verts, err := encodeVertices(in.Vertices)
	if err != nil {
		return nil, err
	}
	var out *Sheet
	err = s.tx(func(t *sql.Tx) error {
		if err := checkFrozen(t, projectID); err != nil {
			return err
		}
		var pos int
		if err := t.QueryRow(`SELECT COALESCE(MAX(position)+1,0) FROM sheets WHERE project_id=?`, projectID).Scan(&pos); err != nil {
			return err
		}
		res, err := t.Exec(`INSERT INTO sheets(project_id,name,vertices,r,g,b,opacity,position)
			VALUES(?,?,?,?,?,?,?,?)`, projectID, in.Name, verts, in.R, in.G, in.B, in.OpacityMillis, pos)
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		if err := invalidate(t, projectID); err != nil {
			return err
		}
		row := t.QueryRow(`SELECT `+sheetCols+` FROM sheets WHERE id=?`, id)
		out, err = scanSheet(row)
		return err
	})
	return out, err
}

func (s *Store) ListSheets(projectID int64) ([]Sheet, error) {
	rows, err := s.db.Query(`SELECT `+sheetCols+` FROM sheets WHERE project_id=? ORDER BY position,id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Sheet
	for rows.Next() {
		sh, err := scanSheet(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *sh)
	}
	return out, rows.Err()
}

func (s *Store) GetSheet(id int64) (*Sheet, error) {
	row := s.db.QueryRow(`SELECT `+sheetCols+` FROM sheets WHERE id=?`, id)
	sh, err := scanSheet(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return sh, err
}

func (s *Store) UpdateSheet(id int64, in SheetInput) (*Sheet, error) {
	if err := validateSheetIn(in); err != nil {
		return nil, err
	}
	verts, err := encodeVertices(in.Vertices)
	if err != nil {
		return nil, err
	}
	var out *Sheet
	err = s.tx(func(t *sql.Tx) error {
		var projectID int64
		if err := t.QueryRow(`SELECT project_id FROM sheets WHERE id=?`, id).Scan(&projectID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if err := checkFrozen(t, projectID); err != nil {
			return err
		}
		if _, err := t.Exec(`UPDATE sheets SET name=?,vertices=?,r=?,g=?,b=?,opacity=? WHERE id=?`,
			in.Name, verts, in.R, in.G, in.B, in.OpacityMillis, id); err != nil {
			return err
		}
		if err := invalidate(t, projectID); err != nil {
			return err
		}
		out, err = scanSheet(t.QueryRow(`SELECT `+sheetCols+` FROM sheets WHERE id=?`, id))
		return err
	})
	return out, err
}

func (s *Store) DeleteSheet(id int64) error {
	return s.tx(func(t *sql.Tx) error {
		var projectID int64
		if err := t.QueryRow(`SELECT project_id FROM sheets WHERE id=?`, id).Scan(&projectID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if err := checkFrozen(t, projectID); err != nil {
			return err
		}
		if _, err := t.Exec(`DELETE FROM sheets WHERE id=?`, id); err != nil {
			return err
		}
		return invalidate(t, projectID)
	})
}
