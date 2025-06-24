package server

import (
	"net/http"

	"github.com/CyberOneHQ/phishrig/internal/server/handlers"
	"github.com/CyberOneHQ/phishrig/internal/server/middleware"
	"github.com/gorilla/mux"
)

func NewRouter(api *handlers.APIHandler) http.Handler {
	r := mux.NewRouter()

	// API routes
	apiRouter := r.PathPrefix("/api").Subrouter()
	apiRouter.HandleFunc("/dashboard", api.HandleDashboard).Methods("GET")
	apiRouter.HandleFunc("/credentials", api.HandleCredentials).Methods("GET")
	apiRouter.HandleFunc("/services", api.HandleServices).Methods("GET")
	apiRouter.HandleFunc("/phishlets", api.HandlePhishlets).Methods("GET")

	// WebSocket
	r.HandleFunc("/ws", api.HandleWebSocket)

	// Dashboard UI - inline HTML for zero-dependency deployment
	r.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(dashboardHTML))
	})

	// Apply middleware
	handler := middleware.RequestLogger(middleware.LocalhostOnly(r))
	return handler
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>PhishRig Dashboard</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#0a0a0f;color:#e0e0e0;min-height:100vh}
.container{max-width:1200px;margin:0 auto;padding:24px}
header{display:flex;align-items:center;justify-content:space-between;margin-bottom:32px;padding-bottom:16px;border-bottom:1px solid #1a1a2e}
h1{font-size:24px;color:#00d4ff;font-weight:600}
.badge{background:#1a1a2e;padding:4px 12px;border-radius:12px;font-size:12px;color:#888}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(300px,1fr));gap:20px;margin-bottom:32px}
.card{background:#12121a;border:1px solid #1a1a2e;border-radius:12px;padding:20px}
.card h2{font-size:14px;color:#888;text-transform:uppercase;letter-spacing:1px;margin-bottom:16px}
.metric{font-size:36px;font-weight:700;color:#00d4ff}
.metric.warning{color:#ff6b35}
.service{display:flex;justify-content:space-between;align-items:center;padding:8px 0;border-bottom:1px solid #1a1a2e}
.service:last-child{border-bottom:none}
.status{display:inline-block;width:8px;height:8px;border-radius:50%;margin-right:8px}
.status.active{background:#00ff88}
.status.inactive{background:#ff4444}
.status.unknown{background:#888}
table{width:100%;border-collapse:collapse}
th,td{text-align:left;padding:10px 12px;border-bottom:1px solid #1a1a2e}
th{color:#888;font-size:12px;text-transform:uppercase;letter-spacing:1px}
td{font-size:14px}
.empty{text-align:center;padding:40px;color:#555}
#error-banner{display:none;background:#2a1a1a;border:1px solid #ff4444;color:#ff4444;padding:12px 20px;border-radius:8px;margin-bottom:20px}
</style>
</head>
<body>
<div class="container">
<header>
<h1>PhishRig</h1>
<span class="badge" id="engagement-badge">No engagement loaded</span>
</header>
<div id="error-banner"></div>
<div class="grid">
<div class="card">
<h2>Captured Credentials</h2>
<div class="metric" id="cred-count">--</div>
</div>
<div class="card">
<h2>Campaigns</h2>
<div class="metric" id="campaign-count">--</div>
</div>
<div class="card">
<h2>Services</h2>
<div id="services-list"></div>
</div>
</div>
<div class="card" style="margin-bottom:20px">
<h2>Available Phishlets</h2>
<div id="phishlets-list"></div>
</div>
<div class="card">
<h2>Captured Credentials</h2>
<table>
<thead><tr><th>Time</th><th>Phishlet</th><th>Username</th><th>Password</th><th>Source IP</th></tr></thead>
<tbody id="creds-table"><tr><td colspan="5" class="empty">No credentials captured yet</td></tr></tbody>
</table>
</div>
</div>
<script>
const $ = s => document.querySelector(s);
const $$ = s => document.querySelectorAll(s);

async function fetchDashboard() {
  try {
    const res = await fetch('/api/dashboard');
    if (!res.ok) throw new Error('API returned ' + res.status);
    const data = await res.json();
    render(data);
    $('#error-banner').style.display = 'none';
  } catch (err) {
    $('#error-banner').textContent = 'Dashboard API error: ' + err.message;
    $('#error-banner').style.display = 'block';
  }
}

function render(d) {
  if (d.engagement) {
    $('#engagement-badge').textContent = d.engagement.name + ' (' + d.engagement.status + ')';
  }
  $('#cred-count').textContent = d.credential_count || 0;
  $('#campaign-count').textContent = d.campaign_count || 0;

  // Services
  let shtml = '';
  (d.services || []).forEach(s => {
    const cls = s.status === 'active' ? 'active' : (s.status === 'inactive' ? 'inactive' : 'unknown');
    shtml += '<div class="service"><span><span class="status ' + cls + '"></span>' + s.name + '</span><span>' + s.status + '</span></div>';
  });
  $('#services-list').innerHTML = shtml || '<div class="empty">No services found</div>';

  // Phishlets
  let phtml = '';
  (d.phishlets || []).forEach(p => {
    phtml += '<div class="service"><span>' + p.name + '</span><span>' + (p.enabled ? 'enabled' : 'available') + '</span></div>';
  });
  $('#phishlets-list').innerHTML = phtml || '<div class="empty">No phishlets found</div>';

  // Credentials table
  const creds = d.credentials || [];
  if (creds.length === 0) {
    $('#creds-table').innerHTML = '<tr><td colspan="5" class="empty">No credentials captured yet</td></tr>';
  } else {
    let rows = '';
    creds.forEach(c => {
      const t = new Date(c.captured_at).toLocaleString();
      rows += '<tr><td>' + t + '</td><td>' + esc(c.phishlet) + '</td><td>' + esc(c.username) + '</td><td>' + esc(c.password) + '</td><td>' + esc(c.remote_addr) + '</td></tr>';
    });
    $('#creds-table').innerHTML = rows;
  }
}

function esc(s) {
  if (!s) return '';
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

// WebSocket for real-time updates
function connectWS() {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  const ws = new WebSocket(proto + '//' + location.host + '/ws');
  ws.onmessage = () => fetchDashboard();
  ws.onclose = () => setTimeout(connectWS, 3000);
}

fetchDashboard();
setInterval(fetchDashboard, 10000);
connectWS();
</script>
</body>
</html>`
