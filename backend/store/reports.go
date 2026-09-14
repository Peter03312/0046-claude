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

// CreateReport stores a run against the current project version.
func (s *Store) CreateReport(projectID int64, status, resultJSON, inputJSON string) (*Report, error) {
	if status != "passed" && status != "failed" {
		return nil, errors.New("status must be passed or failed")
	}
	var out Report
	err := s.tx(func(t *sql.Tx) error {
		var version int64
		if err := t.QueryRow(`SELECT version FROM projects WHERE id=?`, projectID).Scan(&version); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
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
