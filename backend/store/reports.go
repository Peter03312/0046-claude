package store

import (
	"database/sql"
	"errors"
	"time"
)

// Report is a stored proof run.
type Report struct {
	ID          int64   `json:"id"`
	ProjectID   int64   `json:"projectId"`
	Version     int64   `json:"version"`
	Status      string  `json:"status"` // passed | failed
	ResultJSON  string  `json:"-"`
	InputJSON   string  `json:"-"`
	Confirmed   bool    `json:"confirmed"`
	Expired     bool    `json:"expired"`
	CreatedAt   string  `json:"createdAt"`
	ConfirmedAt *string `json:"confirmedAt,omitempty"`
}

// ProofSnapshot is a consistent view of exactly the inputs a proof evaluated:
// project (including its version), all sheets and all acts. The proof result
// is persisted only against SnapshotVersion, never against a version read at
// insert time, which closes the edit-during-proof race.
type ProofSnapshot struct {
	Project Project
	Sheets  []Sheet
	Acts    []Act
}

// ProofSnapshot reads one consistent input snapshot inside a single SQLite
// transaction.
func (s *Store) ProofSnapshot(projectID int64) (*ProofSnapshot, error) {
	t, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer t.Rollback()
	p, err := scanProject(t, projectID)
	if err != nil {
		return nil, err
	}
	rows, err := t.Query(`SELECT `+sheetCols+` FROM sheets WHERE project_id=? ORDER BY position,id`, projectID)
	if err != nil {
		return nil, err
	}
	var sheets []Sheet
	for rows.Next() {
		sh, err := scanSheet(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		sheets = append(sheets, *sh)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	actRows, err := t.Query(`SELECT id,project_id,name,position FROM acts WHERE project_id=? ORDER BY position,id`, projectID)
	if err != nil {
		return nil, err
	}
	var actIDs []int64
	for actRows.Next() {
		var a Act
		if err := actRows.Scan(&a.ID, &a.ProjectID, &a.Name, &a.Position); err != nil {
			actRows.Close()
			return nil, err
		}
		actIDs = append(actIDs, a.ID)
	}
	actRows.Close()
	if err := actRows.Err(); err != nil {
		return nil, err
	}
	acts := make([]Act, 0, len(actIDs))
	for _, id := range actIDs {
		a, err := loadAct(t, id)
		if err != nil {
			return nil, err
		}
		acts = append(acts, *a)
	}
	if sheets == nil {
		sheets = []Sheet{}
	}
	return &ProofSnapshot{Project: *p, Sheets: sheets, Acts: acts}, nil
}

// CreateReportVersioned stores a proof result ONLY if the project version is
// still snapshotVersion. The transaction performs a write first to take the
// SQLite reserved lock, then re-reads the version: any concurrent edit that
// bumped the version serialises against this transaction and the report is
// rejected with ErrStaleReport instead of being stamped onto the new inputs.
func (s *Store) CreateReportVersioned(snapshotVersion int64, projectID int64, status, resultJSON, inputJSON string) (*Report, error) {
	if status != "passed" && status != "failed" {
		return nil, errors.New("status must be passed or failed")
	}
	var out Report
	err := s.tx(func(t *sql.Tx) error {
		// Acquire a write lock before reading so an in-flight edit cannot
		// commit between the version check and the report insert.
		if _, err := t.Exec(`UPDATE projects SET updated_at=updated_at WHERE id=?`, projectID); err != nil {
			return err
		}
		var version int64
		if err := t.QueryRow(`SELECT version FROM projects WHERE id=?`, projectID).
			Scan(&version); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if version != snapshotVersion {
			return ErrStaleReport
		}
		res, err := t.Exec(`INSERT INTO reports(project_id,version,status,result_json,input_json) VALUES(?,?,?,?,?)`,
			projectID, version, status, resultJSON, inputJSON)
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		r, err := s.getReportTx(t, id)
		if err != nil {
			return err
		}
		out = *r
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func scanReportRow(row *sql.Row, r *Report) error {
	var conf, exp int
	var confirmedAt sql.NullString
	err := row.Scan(&r.ID, &r.ProjectID, &r.Version, &r.Status, &r.ResultJSON, &r.InputJSON,
		&conf, &exp, &r.CreatedAt, &confirmedAt)
	r.Confirmed = conf == 1
	r.Expired = exp == 1
	if confirmedAt.Valid {
		r.ConfirmedAt = &confirmedAt.String
	}
	return err
}

func (s *Store) GetReport(id int64) (*Report, error) {
	row := s.db.QueryRow(`SELECT id,project_id,version,status,result_json,input_json,confirmed,expired,created_at,confirmed_at
		FROM reports WHERE id=?`, id)
	var r Report
	err := scanReportRow(row, &r)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) ListReports(projectID int64) ([]Report, error) {
	rows, err := s.db.Query(`SELECT id,project_id,version,status,'','',confirmed,expired,created_at,confirmed_at
		FROM reports WHERE project_id=? ORDER BY id DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Report
	for rows.Next() {
		var r Report
		var conf, exp int
		var confirmedAt sql.NullString
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.Version, &r.Status, &r.ResultJSON, &r.InputJSON,
			&conf, &exp, &r.CreatedAt, &confirmedAt); err != nil {
			return nil, err
		}
		r.Confirmed = conf == 1
		r.Expired = exp == 1
		if confirmedAt.Valid {
			r.ConfirmedAt = &confirmedAt.String
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ConfirmReport freezes the project inputs. Only a passing, current,
// non-expired report can be confirmed; otherwise an error describes why.
func (s *Store) ConfirmReport(id int64) (*Report, error) {
	var out *Report
	err := s.tx(func(t *sql.Tx) error {
		var r Report
		row := t.QueryRow(`SELECT id,project_id,version,status,result_json,input_json,confirmed,expired,created_at,confirmed_at
			FROM reports WHERE id=?`, id)
		if err := scanReportRow(row, &r); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if r.Expired {
			return errors.New("report is expired because inputs changed")
		}
		if r.Status != "passed" {
			return errors.New("only a passing report can be confirmed")
		}
		var currentVersion int64
		var frozen int
		if err := t.QueryRow(`SELECT version,frozen FROM projects WHERE id=?`, r.ProjectID).
			Scan(&currentVersion, &frozen); err != nil {
			return err
		}
		if currentVersion != r.Version {
			return errors.New("report does not match current inputs")
		}
		if _, err := t.Exec(`UPDATE reports SET confirmed=1, confirmed_at=? WHERE id=?`,
			time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), id); err != nil {
			return err
		}
		if _, err := t.Exec(`UPDATE projects SET frozen=1, updated_at=? WHERE id=?`,
			time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), r.ProjectID); err != nil {
			return err
		}
		r2, err := s.getReportTx(t, id)
		if err != nil {
			return err
		}
		out = r2
		return nil
	})
	return out, err
}

func (s *Store) getReportTx(t *sql.Tx, id int64) (*Report, error) {
	row := t.QueryRow(`SELECT id,project_id,version,status,result_json,input_json,confirmed,expired,created_at,confirmed_at
		FROM reports WHERE id=?`, id)
	var r Report
	if err := scanReportRow(row, &r); err != nil {
		return nil, err
	}
	return &r, nil
}
