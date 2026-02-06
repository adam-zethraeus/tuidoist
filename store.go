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
CREATE TABLE IF NOT EXISTS mutation_queue (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	entity_type TEXT NOT NULL,
	entity_id   TEXT NOT NULL,
	action      TEXT NOT NULL,
	payload     TEXT NOT NULL DEFAULT '',
	snapshot    TEXT NOT NULL DEFAULT '',
	status      TEXT NOT NULL DEFAULT 'pending',
	conflict    TEXT NOT NULL DEFAULT '',
	created_at  INTEGER NOT NULL,
	attempts    INTEGER NOT NULL DEFAULT 0
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

// --- Single task lookup ---

// GetTaskByID returns a single cached task by ID.
func (s *Store) GetTaskByID(taskID string) (*Task, error) {
	var blob string
	err := s.db.QueryRow("SELECT data FROM tasks WHERE id = ?", taskID).Scan(&blob)
	if err != nil {
		return nil, err
	}
	var t Task
	if err := json.Unmarshal([]byte(blob), &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// --- Mutation queue ---

// EnqueueMutation inserts a mutation into the queue and returns its ID.
func (s *Store) EnqueueMutation(m Mutation) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO mutation_queue (entity_type, entity_id, action, payload, snapshot, status, conflict, created_at, attempts)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.EntityType, m.EntityID, string(m.Action), m.Payload, m.Snapshot,
		string(m.Status), m.Conflict, m.CreatedAt.Unix(), m.Attempts,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// NextPendingMutation returns the oldest mutation with status=pending.
func (s *Store) NextPendingMutation() (*Mutation, error) {
	row := s.db.QueryRow(
		`SELECT id, entity_type, entity_id, action, payload, snapshot, status, conflict, created_at, attempts
		 FROM mutation_queue WHERE status = 'pending' ORDER BY id ASC LIMIT 1`,
	)
	return scanMutation(row)
}

// UpdateMutationStatus updates a mutation's status and conflict description.
func (s *Store) UpdateMutationStatus(id int64, status MutationStatus, conflict string) error {
	_, err := s.db.Exec(
		"UPDATE mutation_queue SET status = ?, conflict = ? WHERE id = ?",
		string(status), conflict, id,
	)
	return err
}

// IncrementMutationAttempts increments the attempt counter for a mutation.
func (s *Store) IncrementMutationAttempts(id int64) error {
	_, err := s.db.Exec("UPDATE mutation_queue SET attempts = attempts + 1 WHERE id = ?", id)
	return err
}

// DeleteMutation removes a mutation from the queue.
func (s *Store) DeleteMutation(id int64) error {
	_, err := s.db.Exec("DELETE FROM mutation_queue WHERE id = ?", id)
	return err
}

// PendingCount returns the number of pending mutations.
func (s *Store) PendingCount() int {
	var count int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM mutation_queue WHERE status = 'pending'").Scan(&count)
	return count
}

// FlushingCount returns the number of currently-flushing mutations.
func (s *Store) FlushingCount() int {
	var count int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM mutation_queue WHERE status = 'flushing'").Scan(&count)
	return count
}

// ConflictCount returns the number of conflicted mutations.
func (s *Store) ConflictCount() int {
	var count int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM mutation_queue WHERE status = 'conflicted'").Scan(&count)
	return count
}

// GetConflictedMutations returns all conflicted mutations.
func (s *Store) GetConflictedMutations() ([]Mutation, error) {
	return s.queryMutations("SELECT id, entity_type, entity_id, action, payload, snapshot, status, conflict, created_at, attempts FROM mutation_queue WHERE status = 'conflicted' ORDER BY id ASC")
}

// GetAllMutations returns all mutations (for queue view).
func (s *Store) GetAllMutations() ([]Mutation, error) {
	return s.queryMutations("SELECT id, entity_type, entity_id, action, payload, snapshot, status, conflict, created_at, attempts FROM mutation_queue ORDER BY id ASC")
}

func (s *Store) queryMutations(query string) ([]Mutation, error) {
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mutations []Mutation
	for rows.Next() {
		var m Mutation
		var action, status string
		var createdAt int64
		if err := rows.Scan(&m.ID, &m.EntityType, &m.EntityID, &action, &m.Payload, &m.Snapshot, &status, &m.Conflict, &createdAt, &m.Attempts); err != nil {
			return nil, err
		}
		m.Action = MutationAction(action)
		m.Status = MutationStatus(status)
		m.CreatedAt = time.Unix(createdAt, 0)
		mutations = append(mutations, m)
	}
	return mutations, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanMutation(row scannable) (*Mutation, error) {
	var m Mutation
	var action, status string
	var createdAt int64
	err := row.Scan(&m.ID, &m.EntityType, &m.EntityID, &action, &m.Payload, &m.Snapshot, &status, &m.Conflict, &createdAt, &m.Attempts)
	if err != nil {
		return nil, err
	}
	m.Action = MutationAction(action)
	m.Status = MutationStatus(status)
	m.CreatedAt = time.Unix(createdAt, 0)
	return &m, nil
}
