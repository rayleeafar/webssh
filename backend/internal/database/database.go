package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

func Initialize(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return db, nil
}

func migrate(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		encryption_key_salt TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS credentials (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		auth_type TEXT NOT NULL DEFAULT 'password',
		encrypted_value TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_credentials_user_id ON credentials(user_id);

	CREATE TABLE IF NOT EXISTS nodes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		host TEXT NOT NULL,
		port INTEGER NOT NULL DEFAULT 22,
		username TEXT NOT NULL,
		credential_id INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (credential_id) REFERENCES credentials(id) ON DELETE RESTRICT
	);

	CREATE INDEX IF NOT EXISTS idx_nodes_user_id ON nodes(user_id);
	CREATE INDEX IF NOT EXISTS idx_nodes_credential_id ON nodes(credential_id);

	CREATE TABLE IF NOT EXISTS sessions (
		token TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL,
		encryption_key TEXT NOT NULL,
		expires_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
	CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
	`

	_, err := db.Exec(schema)
	if err != nil {
		return err
	}

	// Migration: Move encrypted_credentials from nodes to credentials table
	if err := migrateCredentials(db); err != nil {
		return fmt.Errorf("failed to migrate credentials: %w", err)
	}

	return nil
}

func migrateCredentials(db *sql.DB) error {
	// Check if old schema exists (nodes has encrypted_credentials column)
	var hasOldColumn bool
	row := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('nodes') WHERE name='encrypted_credentials'")
	var count int
	if err := row.Scan(&count); err != nil {
		return err
	}
	hasOldColumn = count > 0

	if !hasOldColumn {
		// Already migrated or fresh install
		return nil
	}

	// Check if credential_id column exists
	row = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('nodes') WHERE name='credential_id'")
	if err := row.Scan(&count); err != nil {
		return err
	}
	hasNewColumn := count > 0

	if hasNewColumn {
		// Migration already done
		return nil
	}

	// Begin migration transaction
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Get all existing nodes with credentials
	rows, err := tx.Query("SELECT id, user_id, auth_type, encrypted_credentials FROM nodes")
	if err != nil {
		// If auth_type doesn't exist, try without it
		rows, err = tx.Query("SELECT id, user_id, 'password', encrypted_credentials FROM nodes")
		if err != nil {
			return err
		}
	}
	defer rows.Close()

	type nodeData struct {
		id                   int
		userID               int
		authType             string
		encryptedCredentials string
	}
	var nodes []nodeData

	for rows.Next() {
		var n nodeData
		if err := rows.Scan(&n.id, &n.userID, &n.authType, &n.encryptedCredentials); err != nil {
			return err
		}
		nodes = append(nodes, n)
	}
	rows.Close()

	// Create credentials table entries and map node_id -> credential_id
	nodeCredMap := make(map[int]int)
	for _, n := range nodes {
		result, err := tx.Exec(
			"INSERT INTO credentials (user_id, auth_type, encrypted_value) VALUES (?, ?, ?)",
			n.userID, n.authType, n.encryptedCredentials,
		)
		if err != nil {
			return err
		}
		credID, err := result.LastInsertId()
		if err != nil {
			return err
		}
		nodeCredMap[n.id] = int(credID)
	}

	// Add credential_id column to nodes
	if _, err := tx.Exec("ALTER TABLE nodes ADD COLUMN credential_id INTEGER"); err != nil {
		return err
	}

	// Update nodes with credential_id
	for nodeID, credID := range nodeCredMap {
		if _, err := tx.Exec("UPDATE nodes SET credential_id = ? WHERE id = ?", credID, nodeID); err != nil {
			return err
		}
	}

	// Drop old columns (SQLite doesn't support DROP COLUMN directly, need to recreate table)
	// For now, just leave them - they'll be ignored

	return tx.Commit()
}
