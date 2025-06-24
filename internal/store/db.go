package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the SQLite database connection
type DB struct {
	conn *sql.DB
}

func Open(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("creating db directory %s: %w", dir, err)
	}

	conn, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening sqlite db: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS engagements (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			client TEXT NOT NULL DEFAULT '',
			operator TEXT NOT NULL DEFAULT '',
			start_date TEXT NOT NULL DEFAULT '',
			end_date TEXT NOT NULL DEFAULT '',
			domain TEXT NOT NULL DEFAULT '',
			phishlet_name TEXT NOT NULL DEFAULT '',
			roe_reference TEXT NOT NULL DEFAULT '',
			notes TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS captured_credentials (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			engagement_id TEXT NOT NULL DEFAULT '',
			session_id TEXT NOT NULL UNIQUE,
			phishlet TEXT NOT NULL DEFAULT '',
			username TEXT NOT NULL DEFAULT '',
			password TEXT NOT NULL DEFAULT '',
			tokens_json TEXT NOT NULL DEFAULT '{}',
			user_agent TEXT NOT NULL DEFAULT '',
			remote_addr TEXT NOT NULL DEFAULT '',
			captured_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (engagement_id) REFERENCES engagements(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_creds_engagement ON captured_credentials(engagement_id)`,
		`CREATE INDEX IF NOT EXISTS idx_creds_session ON captured_credentials(session_id)`,
	}

	for _, m := range migrations {
		if _, err := db.conn.Exec(m); err != nil {
			return fmt.Errorf("executing migration: %w", err)
		}
	}
	return nil
}

// UpsertEngagement creates or updates an engagement record
func (db *DB) UpsertEngagement(e Engagement) error {
	now := time.Now()
	_, err := db.conn.Exec(`
		INSERT INTO engagements (id, name, client, operator, start_date, end_date, domain, phishlet_name, roe_reference, notes, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, client=excluded.client, operator=excluded.operator,
			start_date=excluded.start_date, end_date=excluded.end_date,
			domain=excluded.domain, phishlet_name=excluded.phishlet_name,
			roe_reference=excluded.roe_reference, notes=excluded.notes,
			status=excluded.status, updated_at=?`,
		e.ID, e.Name, e.Client, e.Operator, e.StartDate, e.EndDate,
		e.Domain, e.PhishletName, e.RoEReference, e.Notes, e.Status,
		now, now, now,
	)
	return err
}

// GetEngagement returns an engagement by ID
func (db *DB) GetEngagement(id string) (*Engagement, error) {
	row := db.conn.QueryRow(`SELECT id, name, client, operator, start_date, end_date, domain, phishlet_name, roe_reference, notes, status, created_at, updated_at FROM engagements WHERE id = ?`, id)

	var e Engagement
	err := row.Scan(&e.ID, &e.Name, &e.Client, &e.Operator, &e.StartDate, &e.EndDate, &e.Domain, &e.PhishletName, &e.RoEReference, &e.Notes, &e.Status, &e.CreatedAt, &e.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// GetActiveEngagement returns the first active engagement
func (db *DB) GetActiveEngagement() (*Engagement, error) {
	row := db.conn.QueryRow(`SELECT id, name, client, operator, start_date, end_date, domain, phishlet_name, roe_reference, notes, status, created_at, updated_at FROM engagements WHERE status = 'active' ORDER BY created_at DESC LIMIT 1`)

	var e Engagement
	err := row.Scan(&e.ID, &e.Name, &e.Client, &e.Operator, &e.StartDate, &e.EndDate, &e.Domain, &e.PhishletName, &e.RoEReference, &e.Notes, &e.Status, &e.CreatedAt, &e.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// InsertCredential stores a captured credential (idempotent by session_id)
func (db *DB) InsertCredential(c CapturedCredential) error {
	_, err := db.conn.Exec(`
		INSERT OR IGNORE INTO captured_credentials (engagement_id, session_id, phishlet, username, password, tokens_json, user_agent, remote_addr, captured_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.EngagementID, c.SessionID, c.Phishlet, c.Username, c.Password,
		c.TokensJSON, c.UserAgent, c.RemoteAddr, c.CapturedAt, time.Now(),
	)
	return err
}

// GetCredentials returns all credentials for an engagement
func (db *DB) GetCredentials(engagementID string) ([]CapturedCredential, error) {
	rows, err := db.conn.Query(`
		SELECT id, engagement_id, session_id, phishlet, username, password, tokens_json, user_agent, remote_addr, captured_at, created_at
		FROM captured_credentials WHERE engagement_id = ? ORDER BY captured_at DESC`, engagementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creds []CapturedCredential
	for rows.Next() {
		var c CapturedCredential
		if err := rows.Scan(&c.ID, &c.EngagementID, &c.SessionID, &c.Phishlet, &c.Username, &c.Password, &c.TokensJSON, &c.UserAgent, &c.RemoteAddr, &c.CapturedAt, &c.CreatedAt); err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

// GetAllCredentials returns all credentials across all engagements
func (db *DB) GetAllCredentials() ([]CapturedCredential, error) {
	rows, err := db.conn.Query(`
		SELECT id, engagement_id, session_id, phishlet, username, password, tokens_json, user_agent, remote_addr, captured_at, created_at
		FROM captured_credentials ORDER BY captured_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creds []CapturedCredential
	for rows.Next() {
		var c CapturedCredential
		if err := rows.Scan(&c.ID, &c.EngagementID, &c.SessionID, &c.Phishlet, &c.Username, &c.Password, &c.TokensJSON, &c.UserAgent, &c.RemoteAddr, &c.CapturedAt, &c.CreatedAt); err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

// CredentialCount returns the count of credentials for an engagement
func (db *DB) CredentialCount(engagementID string) (int, error) {
	var count int
	err := db.conn.QueryRow(`SELECT COUNT(*) FROM captured_credentials WHERE engagement_id = ?`, engagementID).Scan(&count)
	return count, err
}
