package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/CyberOneHQ/phishrig/internal/evilginx"
	"github.com/CyberOneHQ/phishrig/internal/gophish"
	"github.com/CyberOneHQ/phishrig/internal/store"
	"github.com/gorilla/websocket"
)

// APIHandler holds dependencies for API route handlers
type APIHandler struct {
	DB       store.Repository
	Evilginx *evilginx.Client
	Gophish  *gophish.Client

	// WebSocket event broadcasting
	wsUpgrader websocket.Upgrader
	wsClients  map[*websocket.Conn]bool
	wsMu       sync.RWMutex
}

func NewAPIHandler(db store.Repository, eg *evilginx.Client, gp *gophish.Client) *APIHandler {
	return &APIHandler{
		DB:       db,
		Evilginx: eg,
		Gophish:  gp,
		wsUpgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // localhost only, no CORS concerns
			},
		},
		wsClients: make(map[*websocket.Conn]bool),
	}
}

// BroadcastEvent sends an event to all connected WebSocket clients
func (h *APIHandler) BroadcastEvent(event any) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("[ws] error marshaling event: %v", err)
		return
	}

	h.wsMu.RLock()
	defer h.wsMu.RUnlock()

	for conn := range h.wsClients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[ws] error writing to client: %v", err)
			conn.Close()
			delete(h.wsClients, conn)
		}
	}
}

// HandleWebSocket upgrades HTTP to WebSocket for real-time events
func (h *APIHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := h.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[ws] upgrade error: %v", err)
		return
	}

	h.wsMu.Lock()
	h.wsClients[conn] = true
	h.wsMu.Unlock()

	log.Printf("[ws] client connected from %s", r.RemoteAddr)

	// Keep connection alive; remove on close
	defer func() {
		h.wsMu.Lock()
		delete(h.wsClients, conn)
		h.wsMu.Unlock()
		conn.Close()
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

// HandleDashboard returns the dashboard summary
func (h *APIHandler) HandleDashboard(w http.ResponseWriter, r *http.Request) {
	eng, err := h.DB.GetActiveEngagement()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get engagement: "+err.Error())
		return
	}

	services := h.getServiceHealth()
	phishlets := h.getPhishletInfo()

	var creds []store.CapturedCredential
	var credCount int
	if eng != nil {
		creds, _ = h.DB.GetCredentials(eng.ID)
		credCount, _ = h.DB.CredentialCount(eng.ID)
	}
	if creds == nil {
		creds = []store.CapturedCredential{}
	}

	var campaignCount int
	if h.Gophish != nil {
		campaigns, err := h.Gophish.GetCampaigns()
		if err == nil {
			campaignCount = len(campaigns)
		}
	}

	summary := store.DashboardSummary{
		Engagement:      eng,
		Services:        services,
		Phishlets:       phishlets,
		Credentials:     creds,
		CredentialCount: credCount,
		CampaignCount:   campaignCount,
	}

	writeJSON(w, http.StatusOK, summary)
}

// HandleCredentials returns all captured credentials
func (h *APIHandler) HandleCredentials(w http.ResponseWriter, r *http.Request) {
	creds, err := h.DB.GetAllCredentials()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get credentials: "+err.Error())
		return
	}
	if creds == nil {
		creds = []store.CapturedCredential{}
	}
	writeJSON(w, http.StatusOK, creds)
}

// HandleServices returns service health status
func (h *APIHandler) HandleServices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.getServiceHealth())
}

// HandlePhishlets returns available phishlet info
func (h *APIHandler) HandlePhishlets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.getPhishletInfo())
}

func (h *APIHandler) getServiceHealth() []store.ServiceHealth {
	services := []store.ServiceHealth{}
	for _, name := range []string{"evilginx", "gophish", "mailhog"} {
		status, _ := getSystemdStatus(name)
		services = append(services, store.ServiceHealth{
			Name:   name,
			Status: status,
		})
	}
	return services
}

func (h *APIHandler) getPhishletInfo() []store.PhishletInfo {
	var infos []store.PhishletInfo
	if h.Evilginx == nil {
		return infos
	}

	names, err := h.Evilginx.ListPhishlets()
	if err != nil {
		return infos
	}
	for _, name := range names {
		infos = append(infos, store.PhishletInfo{
			Name:    name,
			Enabled: false, // Would need to parse Evilginx state to determine
		})
	}
	return infos
}

func getSystemdStatus(service string) (string, error) {
	return evilginx.ServiceStatus(service)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
