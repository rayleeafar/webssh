package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

func Initialize(dbPath string) (*sql.DB, error) {
	if dbPath != ":memory:" {
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create database directory: %w", err)
		}
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
	// 1. Users table (no cross-table dependencies)
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			encryption_key_salt TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		return err
	}

	// 2. Credentials table (depends on users only)
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS credentials (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			auth_type TEXT NOT NULL DEFAULT 'password',
			encrypted_value TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)
	`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_credentials_user_id ON credentials(user_id)`); err != nil {
		return err
	}

	// 3. Nodes table — detect legacy schema before creating credential_id indexes
	if err := migrateNodes(db); err != nil {
		return fmt.Errorf("failed to migrate nodes: %w", err)
	}

	// 4. Sessions table — add csrf_token column if missing
	if err := migrateSessions(db); err != nil {
		return fmt.Errorf("failed to migrate sessions: %w", err)
	}

	// 5. Node sysinfo cache table
	if err := migrateNodeSysInfo(db); err != nil {
		return fmt.Errorf("failed to migrate node_sysinfo: %w", err)
	}

	return nil
}

func migrateNodes(db *sql.DB) error {
	// Check whether the nodes table exists at all
	var tableCount int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='nodes'",
	).Scan(&tableCount); err != nil {
		return err
	}

	if tableCount == 0 {
		// Fresh install: create nodes with the target schema
		if _, err := db.Exec(`
			CREATE TABLE nodes (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL,
				name TEXT NOT NULL,
				host TEXT NOT NULL,
				port INTEGER NOT NULL DEFAULT 22,
				username TEXT NOT NULL,
				credential_id INTEGER NOT NULL,
				proxy_type TEXT NOT NULL DEFAULT '',
				proxy_host TEXT NOT NULL DEFAULT '',
				proxy_port INTEGER NOT NULL DEFAULT 0,
				proxy_username TEXT NOT NULL DEFAULT '',
				proxy_credential_id INTEGER NOT NULL DEFAULT 0,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
				FOREIGN KEY (credential_id) REFERENCES credentials(id) ON DELETE RESTRICT
			)
		`); err != nil {
			return err
		}
		if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_user_id ON nodes(user_id)`); err != nil {
			return err
		}
		if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_credential_id ON nodes(credential_id)`); err != nil {
			return err
		}
		return nil
	}

	// Table exists — check whether it's the legacy schema (has encrypted_credentials)
	var legacyCount int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('nodes') WHERE name='encrypted_credentials'",
	).Scan(&legacyCount); err != nil {
		return err
	}

	if legacyCount == 0 {
		// Already current schema — ensure indexes exist
		if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_user_id ON nodes(user_id)`); err != nil {
			return err
		}
		if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_credential_id ON nodes(credential_id)`); err != nil {
			return err
		}
		// Add any missing proxy columns to existing installations
		proxyColumns := []struct{ name, def string }{
			{"proxy_type", "TEXT NOT NULL DEFAULT ''"},
			{"proxy_host", "TEXT NOT NULL DEFAULT ''"},
			{"proxy_port", "INTEGER NOT NULL DEFAULT 0"},
			{"proxy_username", "TEXT NOT NULL DEFAULT ''"},
			{"proxy_credential_id", "INTEGER NOT NULL DEFAULT 0"},
		}
		for _, col := range proxyColumns {
			var count int
			db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('nodes') WHERE name=?", col.name).Scan(&count)
			if count == 0 {
				db.Exec("ALTER TABLE nodes ADD COLUMN " + col.name + " " + col.def)
			}
		}
		return nil
	}

	// Legacy migration: rebuild nodes inside a transaction
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	type nodeRow struct {
		id        int
		userID    int
		name      string
		host      string
		port      int
		username  string
		authType  string
		encCred   string
		createdAt string
		updatedAt string
	}

	rows, err := tx.Query(`
		SELECT id, user_id, name, host, port, username,
		       COALESCE(auth_type, 'password'),
		       encrypted_credentials,
		       CAST(created_at AS TEXT),
		       CAST(updated_at AS TEXT)
		FROM nodes
	`)
	if err != nil {
		// Fallback: table may not have auth_type column
		rows, err = tx.Query(`
			SELECT id, user_id, name, host, port, username,
			       'password',
			       encrypted_credentials,
			       CAST(created_at AS TEXT),
			       CAST(updated_at AS TEXT)
			FROM nodes
		`)
		if err != nil {
			return err
		}
	}
	defer rows.Close()

	var nodeRows []nodeRow
	for rows.Next() {
		var n nodeRow
		if err := rows.Scan(&n.id, &n.userID, &n.name, &n.host, &n.port,
			&n.username, &n.authType, &n.encCred, &n.createdAt, &n.updatedAt); err != nil {
			return err
		}
		nodeRows = append(nodeRows, n)
	}
	rows.Close()

	// Create one credentials row per legacy node
	nodeCredMap := make(map[int]int64)
	for _, n := range nodeRows {
		result, err := tx.Exec(
			"INSERT INTO credentials (user_id, auth_type, encrypted_value) VALUES (?, ?, ?)",
			n.userID, n.authType, n.encCred,
		)
		if err != nil {
			return err
		}
		credID, err := result.LastInsertId()
		if err != nil {
			return err
		}
		nodeCredMap[n.id] = credID
	}

	// Build new nodes table with the target schema
	if _, err := tx.Exec("DROP TABLE IF EXISTS nodes_new"); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		CREATE TABLE nodes_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			host TEXT NOT NULL,
			port INTEGER NOT NULL DEFAULT 22,
			username TEXT NOT NULL,
			credential_id INTEGER NOT NULL,
			proxy_type TEXT NOT NULL DEFAULT '',
			proxy_host TEXT NOT NULL DEFAULT '',
			proxy_port INTEGER NOT NULL DEFAULT 0,
			proxy_username TEXT NOT NULL DEFAULT '',
			proxy_credential_id INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (credential_id) REFERENCES credentials(id) ON DELETE RESTRICT
		)
	`); err != nil {
		return err
	}

	for _, n := range nodeRows {
		credID := nodeCredMap[n.id]
		if _, err := tx.Exec(`
			INSERT INTO nodes_new
			  (id, user_id, name, host, port, username, credential_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, n.id, n.userID, n.name, n.host, n.port, n.username, credID, n.createdAt, n.updatedAt); err != nil {
			return err
		}
	}

	if _, err := tx.Exec("DROP TABLE nodes"); err != nil {
		return err
	}
	if _, err := tx.Exec("ALTER TABLE nodes_new RENAME TO nodes"); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Indexes must be created after the commit (outside the transaction)
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_user_id ON nodes(user_id)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_credential_id ON nodes(credential_id)`); err != nil {
		return err
	}

	return nil
}

func migrateNodeSysInfo(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS node_sysinfo (
			node_id INTEGER PRIMARY KEY,
			hostname TEXT NOT NULL DEFAULT '',
			os TEXT NOT NULL DEFAULT '',
			kernel TEXT NOT NULL DEFAULT '',
			uptime TEXT NOT NULL DEFAULT '',
			cpu_model TEXT NOT NULL DEFAULT '',
			cpu_cores INTEGER NOT NULL DEFAULT 0,
			load_avg TEXT NOT NULL DEFAULT '',
			mem_total INTEGER NOT NULL DEFAULT 0,
			mem_used INTEGER NOT NULL DEFAULT 0,
			disk_total TEXT NOT NULL DEFAULT '',
			disk_used TEXT NOT NULL DEFAULT '',
			disk_pct TEXT NOT NULL DEFAULT '',
			ip_addr TEXT NOT NULL DEFAULT '',
			collected_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
		)
	`)
	return err
}

func migrateSessions(db *sql.DB) error {
	// Create sessions table if it doesn't exist (includes csrf_token)
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			token TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			encryption_key TEXT NOT NULL,
			csrf_token TEXT NOT NULL DEFAULT '',
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)
	`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at)`); err != nil {
		return err
	}

	// Add csrf_token to existing sessions tables that predate this column
	var csrfColCount int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name='csrf_token'",
	).Scan(&csrfColCount); err != nil {
		return err
	}
	if csrfColCount == 0 {
		if _, err := db.Exec(`ALTER TABLE sessions ADD COLUMN csrf_token TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}

	return nil
}
