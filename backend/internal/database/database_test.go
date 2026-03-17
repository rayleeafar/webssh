package database

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestFreshInstall(t *testing.T) {
	db, err := Initialize(":memory:")
	if err != nil {
		t.Fatalf("fresh install failed: %v", err)
	}
	defer db.Close()

	for _, table := range []string{"users", "credentials", "nodes", "sessions"} {
		var count int
		db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
		if count != 1 {
			t.Errorf("expected table %s to exist", table)
		}
	}

	var credIDCount int
	db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('nodes') WHERE name='credential_id'").Scan(&credIDCount)
	if credIDCount != 1 {
		t.Error("expected credential_id column in nodes")
	}

	var oldColCount int
	db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('nodes') WHERE name='encrypted_credentials'").Scan(&oldColCount)
	if oldColCount != 0 {
		t.Error("expected no encrypted_credentials column on fresh install")
	}

	var csrfCount int
	db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name='csrf_token'").Scan(&csrfCount)
	if csrfCount != 1 {
		t.Error("expected csrf_token column in sessions")
	}
}

func TestLegacySchemaMigration(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}

	// Create legacy schema (pre-credentials-table)
	_, err = db.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			encryption_key_salt TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE nodes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			host TEXT NOT NULL,
			port INTEGER NOT NULL DEFAULT 22,
			username TEXT NOT NULL,
			auth_type TEXT NOT NULL DEFAULT 'password',
			encrypted_credentials TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);

		CREATE TABLE sessions (
			token TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			encryption_key TEXT NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);

		INSERT INTO users (username, password_hash, encryption_key_salt) VALUES ('alice', 'hash', 'salt');
		INSERT INTO nodes (user_id, name, host, port, username, encrypted_credentials)
		  VALUES (1, 'myserver', 'example.com', 22, 'root', 'encrypted_data_here');
	`)
	if err != nil {
		t.Fatalf("failed to create legacy schema: %v", err)
	}

	// Run migration — must not fail on legacy schema
	if err := migrate(db); err != nil {
		t.Fatalf("migration failed on legacy schema: %v", err)
	}

	// nodes.credential_id must exist
	var credIDCount int
	db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('nodes') WHERE name='credential_id'").Scan(&credIDCount)
	if credIDCount != 1 {
		t.Error("expected credential_id column after migration")
	}

	// encrypted_credentials must be gone
	var oldColCount int
	db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('nodes') WHERE name='encrypted_credentials'").Scan(&oldColCount)
	if oldColCount != 0 {
		t.Error("expected encrypted_credentials column to be removed after migration")
	}

	// Node data must be preserved
	var nodeName string
	db.QueryRow("SELECT name FROM nodes WHERE id=1").Scan(&nodeName)
	if nodeName != "myserver" {
		t.Errorf("expected node name 'myserver' got '%s'", nodeName)
	}

	// One credential row must have been created for the one legacy node
	var credCount int
	db.QueryRow("SELECT COUNT(*) FROM credentials").Scan(&credCount)
	if credCount != 1 {
		t.Errorf("expected 1 credential row, got %d", credCount)
	}

	// sessions.csrf_token must exist after migration
	var csrfCount int
	db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name='csrf_token'").Scan(&csrfCount)
	if csrfCount != 1 {
		t.Error("expected csrf_token column in sessions after migration")
	}

	db.Close()
}

// TestIdempotentMigration verifies that running migrate() twice does not fail.
func TestIdempotentMigration(t *testing.T) {
	db, err := Initialize(":memory:")
	if err != nil {
		t.Fatalf("first init failed: %v", err)
	}
	defer db.Close()

	if err := migrate(db); err != nil {
		t.Fatalf("second migrate() call failed: %v", err)
	}
}
