package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("not found")

// ErrFrozen is returned when a frozen (confirmed) project is edited.
var ErrFrozen = errors.New("project inputs are frozen by a confirmed report")

// tx runs fn inside a transaction.
func (s *Store) tx(fn func(*sql.Tx) error) error {
	t, err := s.db.Begin()
	if err != nil {
		return err
	}
	if err := fn(t); err != nil {
		_ = t.Rollback()
		return err
	}
	return t.Commit()
}

func checkFrozen(t *sql.Tx, projectID int64) error {
	var frozen int
	if err := t.QueryRow(`SELECT frozen FROM projects WHERE id=?`, projectID).Scan(&frozen); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if frozen == 1 {
		return ErrFrozen
	}
	return nil
}

// invalidate marks every non-expired report of the project stale and bumps the
// project's input version. It must be called inside a write transaction.
func invalidate(t *sql.Tx, projectID int64) error {
	if _, err := t.Exec(`UPDATE reports SET expired=1 WHERE project_id=? AND expired=0`, projectID); err != nil {
		return err
	}
	_, err := t.Exec(`UPDATE projects
		SET version=version+1, updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?`, projectID)
	return err
}

func encodeVertices(vs []Vertex) (string, error) {
	if len(vs) == 0 {
		return "", fmt.Errorf("vertices required")
	}
	b, err := json.Marshal(vs)
	return string(b), err
}

func decodeVertices(raw string) ([]Vertex, error) {
	var vs []Vertex
	if err := json.Unmarshal([]byte(raw), &vs); err != nil {
		return nil, err
	}
	return vs, nil
}

// ---- projects ----

func (s *Store) CreateProject(name string) (*Project, error) {
	if name == "" {
		return nil, fmt.Errorf("name required")
	}
	var p Project
	err := s.tx(func(t *sql.Tx) error {
		res, err := t.Exec(`INSERT INTO projects(name) VALUES(?)`, name)
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		return t.QueryRow(`SELECT id,name,version,frozen,created_at,updated_at FROM projects WHERE id=?`, id).
			Scan(&p.ID, &p.Name, &p.Version, &scanBool{&p.Frozen}, &p.CreatedAt, &p.UpdatedAt)
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) ListProjects() ([]Project, error) {
	rows, err := s.db.Query(`SELECT id,name,version,frozen,created_at,updated_at FROM projects ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		var p Project
		var f int
		if err := rows.Scan(&p.ID, &p.Name, &p.Version, &f, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.Frozen = f == 1
		out = append(out, p)
	}
	return out, rows.Err()
}

func scanProject(t *sql.Tx, id int64) (*Project, error) {
	var p Project
	var f int
	err := t.QueryRow(`SELECT id,name,version,frozen,created_at,updated_at FROM projects WHERE id=?`, id).
		Scan(&p.ID, &p.Name, &p.Version, &f, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Frozen = f == 1
	return &p, nil
}

func (s *Store) GetProject(id int64) (*Project, error) {
	t, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer t.Rollback()
	return scanProject(t, id)
}

func (s *Store) RenameProject(id int64, name string) error {
	if name == "" {
		return fmt.Errorf("name required")
	}
	_, err := s.db.Exec(`UPDATE projects SET name=? WHERE id=?`, name, id)
	if err != nil {
		return err
	}
	n, err := s.countProjects(id)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) countProjects(id int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM projects WHERE id=?`, id).Scan(&n)
	return n, err
}

func (s *Store) DeleteProject(id int64) error {
	return s.tx(func(t *sql.Tx) error {
		res, err := t.Exec(`DELETE FROM projects WHERE id=?`, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// UnfreezeProject releases a confirmed report's freeze and expires it so the
// child can cut a new storyboard after editing.
func (s *Store) UnfreezeProject(id int64) error {
	return s.tx(func(t *sql.Tx) error {
		p, err := scanProject(t, id)
		if err != nil {
			return err
		}
		if !p.Frozen {
			return nil
		}
		if _, err := t.Exec(`UPDATE projects SET frozen=0,
			updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?`, id); err != nil {
			return err
		}
		_, err = t.Exec(`UPDATE reports SET expired=1 WHERE project_id=? AND confirmed=1`, id)
		return err
	})
}

// scanBool adapts a 0/1 integer column to a bool Scan target.
type scanBool struct{ b *bool }

func (s scanBool) Scan(v any) error {
	switch x := v.(type) {
	case int64:
		*s.b = x == 1
	case int:
		*s.b = x == 1
	default:
		return fmt.Errorf("unexpected bool value %T", v)
	}
	return nil
}
