// Package store persists projects, sheets, acts and proof reports in SQLite.
// All mutating methods that change proof inputs invalidate (expire) existing
// reports, and confirming a passing report freezes the project inputs.
package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Store wraps a SQLite database handle.
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS projects (
  id          INTEGER PRIMARY KEY,
  name        TEXT NOT NULL,
  version     INTEGER NOT NULL DEFAULT 1,
  frozen      INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE TABLE IF NOT EXISTS sheets (
  id          INTEGER PRIMARY KEY,
  project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  vertices    TEXT NOT NULL,
  r           INTEGER NOT NULL,
  g           INTEGER NOT NULL,
  b           INTEGER NOT NULL,
  opacity     INTEGER NOT NULL,
  position    INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS acts (
  id          INTEGER PRIMARY KEY,
  project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  position    INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS act_sheets (
  act_id     INTEGER NOT NULL REFERENCES acts(id) ON DELETE CASCADE,
  sheet_id   INTEGER NOT NULL REFERENCES sheets(id) ON DELETE CASCADE,
  stack      INTEGER NOT NULL,
  rotation   INTEGER NOT NULL,
  tx         INTEGER NOT NULL,
  ty         INTEGER NOT NULL,
  PRIMARY KEY (act_id, sheet_id)
);
CREATE TABLE IF NOT EXISTS regions (
  id          INTEGER PRIMARY KEY,
  act_id      INTEGER NOT NULL REFERENCES acts(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  kind        TEXT NOT NULL,
  vertices    TEXT NOT NULL,
  r           INTEGER NOT NULL DEFAULT 0,
  g           INTEGER NOT NULL DEFAULT 0,
  b           INTEGER NOT NULL DEFAULT 0,
  tolerance   INTEGER NOT NULL DEFAULT 0,
  position    INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS reports (
  id           INTEGER PRIMARY KEY,
  project_id   INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  version      INTEGER NOT NULL,
  status       TEXT NOT NULL,            -- passed | failed
  result_json  TEXT NOT NULL,
  input_json   TEXT NOT NULL,
  confirmed    INTEGER NOT NULL DEFAULT 0,
  expired      INTEGER NOT NULL DEFAULT 0,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  confirmed_at TEXT
);
`

// Open opens (creating if needed) the database at dsn.
func Open(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }
