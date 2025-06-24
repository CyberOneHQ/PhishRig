package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/CyberOneHQ/phishrig/internal/evilginx"
	"github.com/CyberOneHQ/phishrig/internal/store"
)

func setupTestHandler(t *testing.T) (*APIHandler, *store.DB) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	phishletsDir := t.TempDir()
	egClient := evilginx.NewClient(dir, phishletsDir)
	handler := NewAPIHandler(db, egClient, nil)
	return handler, db
}

func TestHandleDashboard_NoEngagement(t *testing.T) {
	handler, _ := setupTestHandler(t)

	req := httptest.NewRequest("GET", "/api/dashboard", nil)
	w := httptest.NewRecorder()
	handler.HandleDashboard(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var summary store.DashboardSummary
	if err := json.NewDecoder(w.Body).Decode(&summary); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if summary.Engagement != nil {
		t.Error("expected nil engagement")
	}
	if len(summary.Services) != 3 {
		t.Errorf("expected 3 services, got %d", len(summary.Services))
	}
	if len(summary.Credentials) != 0 {
		t.Errorf("expected 0 credentials, got %d", len(summary.Credentials))
	}
}

func TestHandleDashboard_WithEngagement(t *testing.T) {
	handler, db := setupTestHandler(t)

	db.UpsertEngagement(store.Engagement{
		ID:     "ENG-001",
		Name:   "Test",
		Domain: "test.com",
		Status: "active",
	})

	req := httptest.NewRequest("GET", "/api/dashboard", nil)
	w := httptest.NewRecorder()
	handler.HandleDashboard(w, req)

	var summary store.DashboardSummary
	json.NewDecoder(w.Body).Decode(&summary)

	if summary.Engagement == nil {
		t.Fatal("expected non-nil engagement")
	}
	if summary.Engagement.Name != "Test" {
		t.Errorf("name = %q, want 'Test'", summary.Engagement.Name)
	}
}

func TestHandleCredentials_Empty(t *testing.T) {
	handler, _ := setupTestHandler(t)

	req := httptest.NewRequest("GET", "/api/credentials", nil)
	w := httptest.NewRecorder()
	handler.HandleCredentials(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var creds []store.CapturedCredential
	json.NewDecoder(w.Body).Decode(&creds)

	if len(creds) != 0 {
		t.Errorf("expected 0 credentials, got %d", len(creds))
	}
}

func TestHandleServices(t *testing.T) {
	handler, _ := setupTestHandler(t)

	req := httptest.NewRequest("GET", "/api/services", nil)
	w := httptest.NewRecorder()
	handler.HandleServices(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var services []store.ServiceHealth
	json.NewDecoder(w.Body).Decode(&services)

	if len(services) != 3 {
		t.Errorf("expected 3 services, got %d", len(services))
	}

	names := map[string]bool{}
	for _, s := range services {
		names[s.Name] = true
	}
	for _, expected := range []string{"evilginx", "gophish", "mailhog"} {
		if !names[expected] {
			t.Errorf("missing service %q", expected)
		}
	}
}

func TestHandlePhishlets_EmptyDir(t *testing.T) {
	handler, _ := setupTestHandler(t)

	req := httptest.NewRequest("GET", "/api/phishlets", nil)
	w := httptest.NewRecorder()
	handler.HandlePhishlets(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}
