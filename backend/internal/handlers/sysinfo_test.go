package handlers

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/webssh/manager/internal/database"
)

// setupSysInfoDB creates an in-memory DB with the full schema and returns a
// SysInfoHandler along with two user IDs.
func setupSysInfoDB(t *testing.T) (*SysInfoHandler, int, int) {
	t.Helper()
	db, err := database.Initialize(":memory:")
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	userAID := createTestUser(t, db, "userA", "passA")
	userBID := createTestUser(t, db, "userB", "passB")

	return NewSysInfoHandler(db), userAID, userBID
}

// insertSysInfoCache inserts a minimal node_sysinfo row for the given nodeID.
func insertSysInfoCache(t *testing.T, h *SysInfoHandler, nodeID int) {
	t.Helper()
	_, err := h.db.Exec(`
		INSERT OR REPLACE INTO node_sysinfo
			(node_id, hostname, os, kernel, uptime, cpu_model, cpu_cores, load_avg,
			 mem_total, mem_used, disk_total, disk_used, disk_pct, ip_addr, collected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		nodeID,
		"host-b", "Linux", "5.15", "up 1 day",
		"Intel", 4, "0.10 0.20 0.30",
		8192, 2048,
		"100G", "20G", "20%",
		"10.0.0.2",
	)
	if err != nil {
		t.Fatalf("failed to insert sysinfo cache: %v", err)
	}
}

// TestSysInfoGetForbiddenForOtherUsersNode verifies that requesting sysinfo
// for a node owned by a different user returns 403.
func TestSysInfoGetForbiddenForOtherUsersNode(t *testing.T) {
	h, userAID, userBID := setupSysInfoDB(t)

	nodeBID := createTestNode(t, h.db, userBID, "nodeB", "10.0.0.2")
	insertSysInfoCache(t, h, nodeBID)

	path := "/api/nodes/" + strconv.Itoa(nodeBID) + "/sysinfo"
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RequestURI = path
	req = addUserToContext(req, userAID, "userA", testEncKeyB64)

	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// TestSysInfoGetCachedForbiddenForOtherUsersNode verifies that requesting
// cached sysinfo for a node owned by a different user returns 403.
func TestSysInfoGetCachedForbiddenForOtherUsersNode(t *testing.T) {
	h, userAID, userBID := setupSysInfoDB(t)

	nodeBID := createTestNode(t, h.db, userBID, "nodeB", "10.0.0.2")
	insertSysInfoCache(t, h, nodeBID)

	path := "/api/nodes/" + strconv.Itoa(nodeBID) + "/sysinfo/cached"
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RequestURI = path
	req = addUserToContext(req, userAID, "userA", testEncKeyB64)

	w := httptest.NewRecorder()
	h.GetCached(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// TestSysInfoGetNotFoundForNonexistentNode verifies that requesting sysinfo
// for a node ID that does not exist returns 404.
func TestSysInfoGetNotFoundForNonexistentNode(t *testing.T) {
	h, userAID, _ := setupSysInfoDB(t)

	path := "/api/nodes/99999/sysinfo"
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RequestURI = path
	req = addUserToContext(req, userAID, "userA", testEncKeyB64)

	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
