package store

import (
	"path/filepath"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestUpsertAndGetEngagement(t *testing.T) {
	db := setupTestDB(t)

	eng := Engagement{
		ID:           "ENG-001",
		Name:         "Test Engagement",
		Client:       "TestCorp",
		Operator:     "op@test.com",
		StartDate:    "2026-03-01",
		EndDate:      "2026-03-31",
		Domain:       "login.test.com",
		PhishletName: "o365",
		Status:       "active",
	}

	if err := db.UpsertEngagement(eng); err != nil {
		t.Fatalf("UpsertEngagement: %v", err)
	}

	got, err := db.GetEngagement("ENG-001")
	if err != nil {
		t.Fatalf("GetEngagement: %v", err)
	}
	if got == nil {
		t.Fatal("GetEngagement returned nil")
	}
	if got.Name != "Test Engagement" {
		t.Errorf("Name = %q, want 'Test Engagement'", got.Name)
	}
	if got.Client != "TestCorp" {
		t.Errorf("Client = %q, want 'TestCorp'", got.Client)
	}
	if got.Status != "active" {
		t.Errorf("Status = %q, want 'active'", got.Status)
	}
}

func TestUpsertEngagement_Update(t *testing.T) {
	db := setupTestDB(t)

	eng := Engagement{ID: "ENG-001", Name: "V1", Status: "active"}
	if err := db.UpsertEngagement(eng); err != nil {
		t.Fatal(err)
	}

	eng.Name = "V2"
	if err := db.UpsertEngagement(eng); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetEngagement("ENG-001")
	if got.Name != "V2" {
		t.Errorf("expected updated name 'V2', got %q", got.Name)
	}
}

func TestGetActiveEngagement(t *testing.T) {
	db := setupTestDB(t)

	// No active engagement
	got, err := db.GetActiveEngagement()
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil for no active engagement")
	}

	// Insert active
	db.UpsertEngagement(Engagement{ID: "ENG-001", Name: "Active", Status: "active"})

	got, err = db.GetActiveEngagement()
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Name != "Active" {
		t.Error("expected active engagement")
	}
}

func TestGetEngagement_NotFound(t *testing.T) {
	db := setupTestDB(t)
	got, err := db.GetEngagement("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent engagement")
	}
}

func TestInsertAndGetCredentials(t *testing.T) {
	db := setupTestDB(t)

	db.UpsertEngagement(Engagement{ID: "ENG-001", Name: "Test", Status: "active"})

	cred := CapturedCredential{
		EngagementID: "ENG-001",
		SessionID:    "sess-001",
		Phishlet:     "o365",
		Username:     "user@test.com",
		Password:     "hunter2",
		TokensJSON:   `{"ESTSAUTH":"abc123"}`,
		UserAgent:    "Mozilla/5.0",
		RemoteAddr:   "1.2.3.4",
		CapturedAt:   time.Now(),
	}

	if err := db.InsertCredential(cred); err != nil {
		t.Fatalf("InsertCredential: %v", err)
	}

	creds, err := db.GetCredentials("ENG-001")
	if err != nil {
		t.Fatalf("GetCredentials: %v", err)
	}
	if len(creds) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(creds))
	}
	if creds[0].Username != "user@test.com" {
		t.Errorf("Username = %q, want 'user@test.com'", creds[0].Username)
	}
	if creds[0].Password != "hunter2" {
		t.Errorf("Password = %q, want 'hunter2'", creds[0].Password)
	}
	if creds[0].Phishlet != "o365" {
		t.Errorf("Phishlet = %q, want 'o365'", creds[0].Phishlet)
	}
}

func TestInsertCredential_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	db.UpsertEngagement(Engagement{ID: "ENG-001", Name: "Test", Status: "active"})

	cred := CapturedCredential{
		EngagementID: "ENG-001",
		SessionID:    "sess-dup",
		Username:     "user@test.com",
		CapturedAt:   time.Now(),
	}

	db.InsertCredential(cred)
	db.InsertCredential(cred) // duplicate - should be ignored

	creds, _ := db.GetCredentials("ENG-001")
	if len(creds) != 1 {
		t.Errorf("expected 1 credential after duplicate insert, got %d", len(creds))
	}
}

func TestCredentialCount(t *testing.T) {
	db := setupTestDB(t)
	db.UpsertEngagement(Engagement{ID: "ENG-001", Name: "Test", Status: "active"})

	count, _ := db.CredentialCount("ENG-001")
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	db.InsertCredential(CapturedCredential{EngagementID: "ENG-001", SessionID: "s1", CapturedAt: time.Now()})
	db.InsertCredential(CapturedCredential{EngagementID: "ENG-001", SessionID: "s2", CapturedAt: time.Now()})

	count, _ = db.CredentialCount("ENG-001")
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestGetAllCredentials(t *testing.T) {
	db := setupTestDB(t)
	db.UpsertEngagement(Engagement{ID: "ENG-001", Name: "Test", Status: "active"})
	db.UpsertEngagement(Engagement{ID: "ENG-002", Name: "Test2", Status: "active"})

	db.InsertCredential(CapturedCredential{EngagementID: "ENG-001", SessionID: "s1", CapturedAt: time.Now()})
	db.InsertCredential(CapturedCredential{EngagementID: "ENG-002", SessionID: "s2", CapturedAt: time.Now()})

	creds, err := db.GetAllCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if len(creds) != 2 {
		t.Errorf("expected 2 total creds, got %d", len(creds))
	}
}
