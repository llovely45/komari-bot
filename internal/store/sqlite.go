package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type ManagedServer struct {
	ServerUUID string
	ServerName string
	CreatedAt  string
	UpdatedAt  string
}

type ReminderState struct {
	ServerUUID     string
	CycleKey       string
	Acknowledged   bool
	LastNotifiedOn string
	UpdatedAt      string
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Init() error {
	schema := []string{
		`PRAGMA journal_mode = WAL;`,
		`PRAGMA foreign_keys = ON;`,
		`CREATE TABLE IF NOT EXISTS managed_servers (
			server_uuid TEXT PRIMARY KEY,
			server_name TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS reminder_state (
			server_uuid TEXT PRIMARY KEY,
			cycle_key TEXT NOT NULL DEFAULT '',
			acknowledged INTEGER NOT NULL DEFAULT 0,
			last_notified_on TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL,
			FOREIGN KEY(server_uuid) REFERENCES managed_servers(server_uuid) ON DELETE CASCADE
		);`,
	}

	for _, stmt := range schema {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) AddManagedServers(servers []ManagedServer) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO managed_servers (server_uuid, server_name, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(server_uuid) DO UPDATE SET
			server_name = excluded.server_name,
			updated_at = excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, server := range servers {
		if _, err := stmt.Exec(server.ServerUUID, server.ServerName, now, now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) UpdateManagedServers(servers []ManagedServer) error {
	if len(servers) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE managed_servers SET server_name = ?, updated_at = ? WHERE server_uuid = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, server := range servers {
		if _, err := stmt.Exec(server.ServerName, now, server.ServerUUID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) ListManagedServers() ([]ManagedServer, error) {
	rows, err := s.db.Query(`
		SELECT server_uuid, server_name, created_at, updated_at
		FROM managed_servers
		ORDER BY server_name COLLATE NOCASE, server_uuid
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []ManagedServer
	for rows.Next() {
		var server ManagedServer
		if err := rows.Scan(&server.ServerUUID, &server.ServerName, &server.CreatedAt, &server.UpdatedAt); err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}
	return servers, rows.Err()
}

func (s *Store) ManagedServerMap() (map[string]ManagedServer, error) {
	servers, err := s.ListManagedServers()
	if err != nil {
		return nil, err
	}

	result := make(map[string]ManagedServer, len(servers))
	for _, server := range servers {
		result[server.ServerUUID] = server
	}
	return result, nil
}

func (s *Store) DeleteManagedServer(serverUUID string) error {
	_, err := s.db.Exec(`DELETE FROM managed_servers WHERE server_uuid = ?`, serverUUID)
	return err
}

func (s *Store) GetReminderState(serverUUID string) (ReminderState, bool, error) {
	var state ReminderState
	err := s.db.QueryRow(`
		SELECT server_uuid, cycle_key, acknowledged, last_notified_on, updated_at
		FROM reminder_state
		WHERE server_uuid = ?
	`, serverUUID).Scan(&state.ServerUUID, &state.CycleKey, &state.Acknowledged, &state.LastNotifiedOn, &state.UpdatedAt)
	if err == sql.ErrNoRows {
		return ReminderState{}, false, nil
	}
	if err != nil {
		return ReminderState{}, false, err
	}
	return state, true, nil
}

func (s *Store) SaveReminderState(state ReminderState) error {
	if state.ServerUUID == "" {
		return fmt.Errorf("server uuid is required")
	}

	if state.UpdatedAt == "" {
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	acknowledged := 0
	if state.Acknowledged {
		acknowledged = 1
	}

	_, err := s.db.Exec(`
		INSERT INTO reminder_state (server_uuid, cycle_key, acknowledged, last_notified_on, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(server_uuid) DO UPDATE SET
			cycle_key = excluded.cycle_key,
			acknowledged = excluded.acknowledged,
			last_notified_on = excluded.last_notified_on,
			updated_at = excluded.updated_at
	`, state.ServerUUID, state.CycleKey, acknowledged, state.LastNotifiedOn, state.UpdatedAt)
	return err
}

func (s *Store) AcknowledgeReminder(serverUUID, cycleKey string) (bool, error) {
	result, err := s.db.Exec(`
		UPDATE reminder_state
		SET acknowledged = 1, updated_at = ?
		WHERE server_uuid = ? AND cycle_key = ?
	`, time.Now().UTC().Format(time.RFC3339), serverUUID, cycleKey)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}
