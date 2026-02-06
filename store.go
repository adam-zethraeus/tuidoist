package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Store is a SQLite-backed cache for Todoist data.
type Store struct {
	db  *sql.DB
	ttl time.Duration
}

// NewStore opens (or creates) a SQLite database at dbPath and runs migrations.
func NewStore(dbPath string, ttl time.Duration) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &Store{db: db, ttl: ttl}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func migrate(db *sql.DB) error {
	ddl := `
CREATE TABLE IF NOT EXISTS sync_meta (
	resource_type TEXT,
	scope_id      TEXT DEFAULT '',
	last_synced   INTEGER,
	PRIMARY KEY (resource_type, scope_id)
);
CREATE TABLE IF NOT EXISTS projects (
	id   TEXT PRIMARY KEY,
	data TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS tasks (
	id         TEXT PRIMARY KEY,
	project_id TEXT NOT NULL,
	data       TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sections (
	id         TEXT PRIMARY KEY,
	project_id TEXT NOT NULL,
	data       TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS labels (
	id   TEXT PRIMARY KEY,
	data TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_tasks_project ON tasks(project_id);
CREATE INDEX IF NOT EXISTS idx_sections_project ON sections(project_id);
`
	_, err := db.Exec(ddl)
	return err
}

// --- Staleness ---

// IsStale returns true if the given resource/scope has not been synced within the TTL.
func (s *Store) IsStale(resourceType, scopeID string) bool {
	var ts int64
	err := s.db.QueryRow(
		"SELECT last_synced FROM sync_meta WHERE resource_type = ? AND scope_id = ?",
		resourceType, scopeID,
	).Scan(&ts)
	if err != nil {
		return true
	}
	return time.Since(time.Unix(ts, 0)) > s.ttl
}

// TouchSync records that the given resource/scope was just synced.
func (s *Store) TouchSync(resourceType, scopeID string) {
	_, _ = s.db.Exec(
		"INSERT INTO sync_meta (resource_type, scope_id, last_synced) VALUES (?, ?, ?) "+
			"ON CONFLICT(resource_type, scope_id) DO UPDATE SET last_synced = excluded.last_synced",
		resourceType, scopeID, time.Now().Unix(),
	)
}

// --- Projects ---

// GetProjects returns all cached projects.
func (s *Store) GetProjects() ([]Project, error) {
	rows, err := s.db.Query("SELECT data FROM projects")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var blob string
		if err := rows.Scan(&blob); err != nil {
			return nil, err
		}
		var p Project
		if err := json.Unmarshal([]byte(blob), &p); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// ReplaceProjects replaces all cached projects in a transaction.
func (s *Store) ReplaceProjects(projects []Project) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM projects"); err != nil {
		return err
	}

	stmt, err := tx.Prepare("INSERT INTO projects (id, data) VALUES (?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, p := range projects {
		blob, err := json.Marshal(p)
		if err != nil {
			return err
		}
		if _, err := stmt.Exec(p.ID, string(blob)); err != nil {
			return err
		}
	}

	s.TouchSync("projects", "")
	return tx.Commit()
}

// --- Tasks ---

// GetTasks returns cached tasks for a project.
func (s *Store) GetTasks(projectID string) ([]Task, error) {
	rows, err := s.db.Query("SELECT data FROM tasks WHERE project_id = ?", projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var blob string
		if err := rows.Scan(&blob); err != nil {
			return nil, err
		}
		var t Task
		if err := json.Unmarshal([]byte(blob), &t); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// ReplaceTasks replaces all cached tasks for a project in a transaction.
func (s *Store) ReplaceTasks(projectID string, tasks []Task) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM tasks WHERE project_id = ?", projectID); err != nil {
		return err
	}

	stmt, err := tx.Prepare("INSERT INTO tasks (id, project_id, data) VALUES (?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, t := range tasks {
		blob, err := json.Marshal(t)
		if err != nil {
			return err
		}
		if _, err := stmt.Exec(t.ID, projectID, string(blob)); err != nil {
			return err
		}
	}

	s.TouchSync("tasks", projectID)
	return tx.Commit()
}

// UpsertTask inserts or updates a single task in the cache.
func (s *Store) UpsertTask(t Task) error {
	blob, err := json.Marshal(t)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		"INSERT INTO tasks (id, project_id, data) VALUES (?, ?, ?) "+
			"ON CONFLICT(id) DO UPDATE SET project_id = excluded.project_id, data = excluded.data",
		t.ID, t.ProjectID, string(blob),
	)
	return err
}

// DeleteTask removes a single task from the cache.
func (s *Store) DeleteTask(taskID string) error {
	_, err := s.db.Exec("DELETE FROM tasks WHERE id = ?", taskID)
	return err
}

// --- Sections ---

// GetSections returns cached sections for a project.
func (s *Store) GetSections(projectID string) ([]Section, error) {
	rows, err := s.db.Query("SELECT data FROM sections WHERE project_id = ?", projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sections []Section
	for rows.Next() {
		var blob string
		if err := rows.Scan(&blob); err != nil {
			return nil, err
		}
		var sec Section
		if err := json.Unmarshal([]byte(blob), &sec); err != nil {
			return nil, err
		}
		sections = append(sections, sec)
	}
	return sections, rows.Err()
}

// ReplaceSections replaces all cached sections for a project in a transaction.
func (s *Store) ReplaceSections(projectID string, sections []Section) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM sections WHERE project_id = ?", projectID); err != nil {
		return err
	}

	stmt, err := tx.Prepare("INSERT INTO sections (id, project_id, data) VALUES (?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, sec := range sections {
		blob, err := json.Marshal(sec)
		if err != nil {
			return err
		}
		if _, err := stmt.Exec(sec.ID, projectID, string(blob)); err != nil {
			return err
		}
	}

	s.TouchSync("sections", projectID)
	return tx.Commit()
}
