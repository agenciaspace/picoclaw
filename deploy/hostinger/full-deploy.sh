#!/usr/bin/env bash
# ============================================================
# PicoClaw - Full Hostinger VPS Deploy (All-in-One)
# ============================================================
# This script does EVERYTHING via SSH in a single run:
#   1. Server setup (packages, Docker, firewall, fail2ban)
#   2. User & directory creation
#   3. Configuration files (prompts for API keys)
#   4. Source code sync & Docker build
#   5. Start service & health check verification
#
# Usage:
#   ./deploy/hostinger/full-deploy.sh -h YOUR_VPS_IP [OPTIONS]
#
# Examples:
#   # Interactive (prompts for API keys):
#   ./deploy/hostinger/full-deploy.sh -h 149.28.10.50
#
#   # Non-interactive with env vars:
#   LLM_PROVIDER=anthropic \
#   LLM_API_KEY=sk-ant-xxx \
#   CHAT_CHANNEL=telegram \
#   CHAT_TOKEN=123456:ABC \
#   ./deploy/hostinger/full-deploy.sh -h 149.28.10.50
#
# Options:
#   -h, --host HOST       VPS IP or hostname (required)
#   -u, --user USER       SSH user (default: root)
#   -k, --key  KEY        SSH key path (default: ~/.ssh/id_rsa)
#   -p, --port PORT       SSH port (default: 22)
#   -m, --method METHOD   "docker" or "binary" (default: docker)
#   --skip-setup          Skip server provisioning (if already done)
#   --skip-config         Skip config prompts (use existing server config)
#   --yes                 Skip confirmation prompts
#   --help                Show this help
# ============================================================

set -euo pipefail

# ── Colors ───────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

log()     { echo -e "${GREEN}>>>${NC} $*"; }
warn()    { echo -e "${YELLOW}[!]${NC} $*"; }
error()   { echo -e "${RED}[ERROR]${NC} $*"; exit 1; }
info()    { echo -e "${BLUE}[i]${NC} $*"; }
step()    { echo -e "\n${CYAN}${BOLD}── $1 ──${NC}"; }
success() { echo -e "${GREEN}${BOLD}$*${NC}"; }

# ── Default Configuration ────────────────────────────
HOST=""
SSH_USER="root"
SSH_KEY="${HOME}/.ssh/id_rsa"
SSH_PORT="22"
METHOD="docker"
SKIP_SETUP=false
SKIP_CONFIG=false
AUTO_YES=false
REMOTE_DIR="/opt/picoclaw"

# Pre-set via env (for non-interactive deploys)
LLM_PROVIDER="${LLM_PROVIDER:-}"
LLM_API_KEY="${LLM_API_KEY:-}"
CHAT_CHANNEL="${CHAT_CHANNEL:-}"
CHAT_TOKEN="${CHAT_TOKEN:-}"
CHAT_ALLOW_FROM="${CHAT_ALLOW_FROM:-}"
DEPLOY_TIMEZONE="${DEPLOY_TIMEZONE:-America/Sao_Paulo}"

# ── Parse Arguments ──────────────────────────────────
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--host)      HOST="$2"; shift 2 ;;
        -u|--user)      SSH_USER="$2"; shift 2 ;;
        -k|--key)       SSH_KEY="$2"; shift 2 ;;
        -p|--port)      SSH_PORT="$2"; shift 2 ;;
        -m|--method)    METHOD="$2"; shift 2 ;;
        --skip-setup)   SKIP_SETUP=true; shift ;;
        --skip-config)  SKIP_CONFIG=true; shift ;;
        --yes)          AUTO_YES=true; shift ;;
        --help)
            sed -n '2,/^# =====/p' "$0" | head -n -1 | sed 's/^# //' | sed 's/^#//'
            exit 0
            ;;
        *) error "Unknown option: $1. Use --help for usage." ;;
    esac
done

# ── Validate ─────────────────────────────────────────
[ -z "${HOST}" ] && error "Host is required. Usage: $0 -h YOUR_VPS_IP"

if [ ! -f "${SSH_KEY}" ]; then
    warn "SSH key not found at ${SSH_KEY}"
    info "Will attempt password-based SSH. For better security, set up SSH keys."
    SSH_OPTS="-o StrictHostKeyChecking=accept-new -o ConnectTimeout=15 -p ${SSH_PORT}"
    SCP_OPTS="-o StrictHostKeyChecking=accept-new -o ConnectTimeout=15 -P ${SSH_PORT}"
else
    SSH_OPTS="-o StrictHostKeyChecking=accept-new -o ConnectTimeout=15 -p ${SSH_PORT} -i ${SSH_KEY}"
    SCP_OPTS="-o StrictHostKeyChecking=accept-new -o ConnectTimeout=15 -P ${SSH_PORT} -i ${SSH_KEY}"
fi

SSH_CMD="ssh ${SSH_OPTS} ${SSH_USER}@${HOST}"
SCP_CMD="scp ${SCP_OPTS}"

# ── Get project root ─────────────────────────────────
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# ── Banner ───────────────────────────────────────────
echo ""
echo -e "${CYAN}${BOLD}"
echo "  ╔═══════════════════════════════════════════╗"
echo "  ║     PicoClaw - Hostinger Full Deploy      ║"
echo "  ╚═══════════════════════════════════════════╝"
echo -e "${NC}"
echo "  Host:     ${HOST}"
echo "  User:     ${SSH_USER}"
echo "  Method:   ${METHOD}"
echo "  SSH Key:  ${SSH_KEY}"
echo ""

if [ "${AUTO_YES}" = false ]; then
    read -rp "  Continue with deployment? [Y/n] " confirm
    case "${confirm}" in
        [nN]*) echo "Aborted."; exit 0 ;;
    esac
fi

DEPLOY_START=$(date +%s)

# ════════════════════════════════════════════════════
# PHASE 1: Verify SSH Connection
# ════════════════════════════════════════════════════
step "Phase 1/5: Checking SSH connection"

if ${SSH_CMD} "echo 'ok'" &>/dev/null; then
    log "SSH connection to ${SSH_USER}@${HOST}:${SSH_PORT} OK"
else
    error "Cannot connect via SSH to ${SSH_USER}@${HOST}:${SSH_PORT}

  Troubleshooting:
  - Verify the VPS IP in Hostinger hPanel
  - Enable SSH access in hPanel > Advanced > SSH Access
  - Check your SSH key: ssh-keygen -t ed25519 && ssh-copy-id ${SSH_USER}@${HOST}
  - Try with password: $0 -h ${HOST} (without -k flag)"
fi

# Detect remote OS
REMOTE_OS=$(${SSH_CMD} "cat /etc/os-release 2>/dev/null | head -3 || echo 'unknown'" 2>/dev/null)
info "Remote OS: $(echo "${REMOTE_OS}" | grep PRETTY_NAME | cut -d= -f2 | tr -d '"' || echo 'detected')"

# ════════════════════════════════════════════════════
# PHASE 2: Server Provisioning
# ════════════════════════════════════════════════════
if [ "${SKIP_SETUP}" = false ]; then
    step "Phase 2/5: Server provisioning"
    log "Installing packages, Docker, firewall, fail2ban..."

    ${SSH_CMD} "bash -s -- ${METHOD}" <<'SETUP_SCRIPT'
#!/bin/bash
set -euo pipefail

METHOD="${1:-docker}"

echo "[REMOTE] Updating system packages..."
export DEBIAN_FRONTEND=noninteractive

if command -v apt-get &>/dev/null; then
    apt-get update -qq
    apt-get upgrade -y -qq
    apt-get install -y -qq curl wget git ufw fail2ban unzip jq rsync
elif command -v yum &>/dev/null; then
    yum update -y -q
    yum install -y -q curl wget git firewalld fail2ban unzip jq rsync
elif command -v dnf &>/dev/null; then
    dnf update -y -q
    dnf install -y -q curl wget git firewalld fail2ban unzip jq rsync
fi

# Create dedicated user
if ! id picoclaw &>/dev/null; then
    echo "[REMOTE] Creating picoclaw user..."
    useradd --system --create-home --home-dir /opt/picoclaw --shell /bin/bash picoclaw
fi

# Directory structure
echo "[REMOTE] Creating directories..."
mkdir -p /opt/picoclaw/{bin,config,workspace,logs,backups,src}
chown -R picoclaw:picoclaw /opt/picoclaw

# Docker
if [ "${METHOD}" = "docker" ]; then
    if ! command -v docker &>/dev/null; then
        echo "[REMOTE] Installing Docker..."
        curl -fsSL https://get.docker.com | sh
        systemctl enable docker
        systemctl start docker
        usermod -aG docker picoclaw
    fi
    echo "[REMOTE] Docker: $(docker --version 2>/dev/null)"

    if ! docker compose version &>/dev/null; then
        echo "[REMOTE] Installing Docker Compose..."
        apt-get install -y -qq docker-compose-plugin 2>/dev/null || true
    fi
    echo "[REMOTE] Docker Compose: $(docker compose version 2>/dev/null || echo 'not available')"
else
    # Binary method: install Go
    if ! command -v go &>/dev/null; then
        echo "[REMOTE] Installing Go..."
        GO_VERSION="1.23.4"
        ARCH=$(uname -m)
        case "${ARCH}" in
            x86_64)  GO_ARCH="amd64" ;;
            aarch64) GO_ARCH="arm64" ;;
            *)       GO_ARCH="${ARCH}" ;;
        esac
        curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz" -o /tmp/go.tar.gz
        rm -rf /usr/local/go
        tar -C /usr/local -xzf /tmp/go.tar.gz
        rm /tmp/go.tar.gz
        echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/go.sh
        export PATH=$PATH:/usr/local/go/bin
    fi
    echo "[REMOTE] Go: $(go version 2>/dev/null || echo 'installed')"

    if ! command -v make &>/dev/null; then
        apt-get install -y -qq make 2>/dev/null || yum install -y -q make 2>/dev/null || true
    fi

    # Install systemd service
    cat > /etc/systemd/system/picoclaw.service <<'SVCEOF'
[Unit]
Description=PicoClaw AI Assistant Gateway
Documentation=https://github.com/agenciaspace/picoclaw
After=network-online.target
Wants=network-online.target
StartLimitIntervalSec=300
StartLimitBurst=5

[Service]
Type=simple
User=picoclaw
Group=picoclaw
WorkingDirectory=/opt/picoclaw
ExecStart=/opt/picoclaw/bin/picoclaw gateway
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=10
TimeoutStopSec=30

StandardOutput=append:/opt/picoclaw/logs/picoclaw.log
StandardError=append:/opt/picoclaw/logs/picoclaw-error.log

EnvironmentFile=-/opt/picoclaw/config/.env
Environment=HOME=/opt/picoclaw
Environment=PICOCLAW_CONFIG=/opt/picoclaw/config/config.json

NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/opt/picoclaw
PrivateTmp=yes
ProtectKernelModules=yes
ProtectKernelTunables=yes
ProtectControlGroups=yes
RestrictSUIDSGID=yes
RestrictNamespaces=yes

[Install]
WantedBy=multi-user.target
SVCEOF
    systemctl daemon-reload
    systemctl enable picoclaw
fi

# Firewall
echo "[REMOTE] Configuring firewall..."
if command -v ufw &>/dev/null; then
    ufw --force reset >/dev/null 2>&1
    ufw default deny incoming >/dev/null 2>&1
    ufw default allow outgoing >/dev/null 2>&1
    ufw allow ssh >/dev/null 2>&1
    ufw allow 18790/tcp >/dev/null 2>&1
    ufw --force enable >/dev/null 2>&1
    echo "[REMOTE] UFW: enabled (SSH + 18790)"
elif command -v firewall-cmd &>/dev/null; then
    systemctl enable firewalld >/dev/null 2>&1
    systemctl start firewalld >/dev/null 2>&1
    firewall-cmd --permanent --add-service=ssh >/dev/null 2>&1
    firewall-cmd --permanent --add-port=18790/tcp >/dev/null 2>&1
    firewall-cmd --reload >/dev/null 2>&1
    echo "[REMOTE] firewalld: enabled (SSH + 18790)"
fi

# fail2ban
systemctl enable fail2ban >/dev/null 2>&1
systemctl start fail2ban >/dev/null 2>&1
echo "[REMOTE] fail2ban: active"

# Logrotate
cat > /etc/logrotate.d/picoclaw <<'LOGEOF'
/opt/picoclaw/logs/*.log {
    daily
    missingok
    rotate 14
    compress
    delaycompress
    notifempty
    create 0640 picoclaw picoclaw
}
LOGEOF

echo "[REMOTE] Server provisioning complete!"
SETUP_SCRIPT

    log "Server provisioning done"
else
    step "Phase 2/5: Server provisioning (SKIPPED)"
fi

# ════════════════════════════════════════════════════
# PHASE 3: Configuration
# ════════════════════════════════════════════════════
if [ "${SKIP_CONFIG}" = false ]; then
    step "Phase 3/5: Configuration"

    # Collect config interactively if not provided via env
    if [ -z "${LLM_PROVIDER}" ]; then
        echo ""
        echo "  Which LLM provider will you use?"
        echo "    1) anthropic  (Claude)"
        echo "    2) openai     (GPT)"
        echo "    3) openrouter (Multiple models)"
        echo "    4) gemini     (Google)"
        echo "    5) zhipu      (GLM)"
        echo "    6) groq       (Fast inference)"
        echo "    7) ollama     (Local/self-hosted)"
        echo ""
        read -rp "  Provider [1-7]: " provider_choice
        case "${provider_choice}" in
            1) LLM_PROVIDER="anthropic" ;;
            2) LLM_PROVIDER="openai" ;;
            3) LLM_PROVIDER="openrouter" ;;
            4) LLM_PROVIDER="gemini" ;;
            5) LLM_PROVIDER="zhipu" ;;
            6) LLM_PROVIDER="groq" ;;
            7) LLM_PROVIDER="ollama" ;;
            *) LLM_PROVIDER="anthropic" ;;
        esac
    fi

    if [ -z "${LLM_API_KEY}" ] && [ "${LLM_PROVIDER}" != "ollama" ]; then
        read -rp "  ${LLM_PROVIDER} API key: " LLM_API_KEY
        [ -z "${LLM_API_KEY}" ] && warn "No API key provided. You can set it later on the server."
    fi

    if [ -z "${CHAT_CHANNEL}" ]; then
        echo ""
        echo "  Which chat channel? (optional, press Enter to skip)"
        echo "    1) telegram"
        echo "    2) discord"
        echo "    3) slack"
        echo "    4) whatsapp"
        echo "    5) line"
        echo "    6) none (gateway API only)"
        echo ""
        read -rp "  Channel [1-6]: " channel_choice
        case "${channel_choice}" in
            1) CHAT_CHANNEL="telegram" ;;
            2) CHAT_CHANNEL="discord" ;;
            3) CHAT_CHANNEL="slack" ;;
            4) CHAT_CHANNEL="whatsapp" ;;
            5) CHAT_CHANNEL="line" ;;
            *) CHAT_CHANNEL="" ;;
        esac
    fi

    if [ -n "${CHAT_CHANNEL}" ] && [ -z "${CHAT_TOKEN}" ]; then
        read -rp "  ${CHAT_CHANNEL} bot token: " CHAT_TOKEN
    fi

    if [ -n "${CHAT_CHANNEL}" ] && [ -z "${CHAT_ALLOW_FROM}" ]; then
        read -rp "  Allowed user IDs (comma-separated, or * for all): " CHAT_ALLOW_FROM
    fi

    # Determine model based on provider
    case "${LLM_PROVIDER}" in
        anthropic)   DEFAULT_MODEL="claude-sonnet-4-20250514" ;;
        openai)      DEFAULT_MODEL="gpt-4o" ;;
        openrouter)  DEFAULT_MODEL="anthropic/claude-sonnet-4" ;;
        gemini)      DEFAULT_MODEL="gemini-2.0-flash" ;;
        zhipu)       DEFAULT_MODEL="glm-4" ;;
        groq)        DEFAULT_MODEL="llama-3.3-70b-versatile" ;;
        ollama)      DEFAULT_MODEL="llama3.2" ;;
        *)           DEFAULT_MODEL="claude-sonnet-4-20250514" ;;
    esac

    # Build env key name
    case "${LLM_PROVIDER}" in
        anthropic)   ENV_KEY_NAME="ANTHROPIC_API_KEY" ;;
        openai)      ENV_KEY_NAME="OPENAI_API_KEY" ;;
        openrouter)  ENV_KEY_NAME="OPENROUTER_API_KEY" ;;
        gemini)      ENV_KEY_NAME="GEMINI_API_KEY" ;;
        zhipu)       ENV_KEY_NAME="ZHIPU_API_KEY" ;;
        groq)        ENV_KEY_NAME="GROQ_API_KEY" ;;
        ollama)      ENV_KEY_NAME="" ;;
        *)           ENV_KEY_NAME="${LLM_PROVIDER^^}_API_KEY" ;;
    esac

    # Build chat token env name
    case "${CHAT_CHANNEL}" in
        telegram) CHAT_ENV_KEY="TELEGRAM_BOT_TOKEN" ;;
        discord)  CHAT_ENV_KEY="DISCORD_BOT_TOKEN" ;;
        slack)    CHAT_ENV_KEY="SLACK_BOT_TOKEN" ;;
        *)        CHAT_ENV_KEY="" ;;
    esac

    # Format allow_from as JSON array
    if [ -n "${CHAT_ALLOW_FROM}" ] && [ "${CHAT_ALLOW_FROM}" != "*" ]; then
        ALLOW_FROM_JSON=$(echo "${CHAT_ALLOW_FROM}" | tr ',' '\n' | sed 's/^ *//;s/ *$//' | awk '{printf "\"%s\",", $0}' | sed 's/,$//')
        ALLOW_FROM_JSON="[${ALLOW_FROM_JSON}]"
    else
        ALLOW_FROM_JSON="[]"
    fi

    log "Writing configuration to server..."

    # Write .env file
    ${SSH_CMD} "cat > /opt/picoclaw/config/.env" <<ENVEOF
# PicoClaw Production Environment
# Generated: $(date -Iseconds)

# LLM Provider
${ENV_KEY_NAME:+${ENV_KEY_NAME}=${LLM_API_KEY}}

# Chat Channel
${CHAT_ENV_KEY:+${CHAT_ENV_KEY}=${CHAT_TOKEN}}

# Timezone
TZ=${DEPLOY_TIMEZONE}
ENVEOF

    # Write config.json
    CHANNEL_ENABLED="false"
    [ -n "${CHAT_CHANNEL}" ] && CHANNEL_ENABLED="true"

    ${SSH_CMD} "cat > /opt/picoclaw/config/config.json" <<CFGEOF
{
  "agents": {
    "defaults": {
      "workspace": "/opt/picoclaw/workspace",
      "restrict_to_workspace": true,
      "model": "${DEFAULT_MODEL}",
      "max_tokens": 8192,
      "temperature": 0.7,
      "max_tool_iterations": 20
    }
  },
  "channels": {
    "telegram": {
      "enabled": $([ "${CHAT_CHANNEL}" = "telegram" ] && echo "true" || echo "false"),
      "token": "$([ "${CHAT_CHANNEL}" = "telegram" ] && echo "${CHAT_TOKEN}" || echo "")",
      "allow_from": $([ "${CHAT_CHANNEL}" = "telegram" ] && echo "${ALLOW_FROM_JSON}" || echo "[]")
    },
    "discord": {
      "enabled": $([ "${CHAT_CHANNEL}" = "discord" ] && echo "true" || echo "false"),
      "token": "$([ "${CHAT_CHANNEL}" = "discord" ] && echo "${CHAT_TOKEN}" || echo "")",
      "allow_from": $([ "${CHAT_CHANNEL}" = "discord" ] && echo "${ALLOW_FROM_JSON}" || echo "[]")
    },
    "slack": {
      "enabled": $([ "${CHAT_CHANNEL}" = "slack" ] && echo "true" || echo "false"),
      "bot_token": "$([ "${CHAT_CHANNEL}" = "slack" ] && echo "${CHAT_TOKEN}" || echo "")",
      "app_token": "",
      "allow_from": $([ "${CHAT_CHANNEL}" = "slack" ] && echo "${ALLOW_FROM_JSON}" || echo "[]")
    }
  },
  "providers": {
    "${LLM_PROVIDER}": {
      "api_key": "${LLM_API_KEY}",
      "api_base": ""
    }
  },
  "heartbeat": {
    "enabled": true,
    "interval": 30
  },
  "gateway": {
    "host": "0.0.0.0",
    "port": 18790
  }
}
CFGEOF

    # Secure config files
    ${SSH_CMD} "chown picoclaw:picoclaw /opt/picoclaw/config/.env /opt/picoclaw/config/config.json && chmod 600 /opt/picoclaw/config/.env /opt/picoclaw/config/config.json"

    log "Configuration written"
    info "Provider: ${LLM_PROVIDER} (model: ${DEFAULT_MODEL})"
    [ -n "${CHAT_CHANNEL}" ] && info "Channel: ${CHAT_CHANNEL} (enabled)" || info "Channel: none (gateway API only)"
else
    step "Phase 3/5: Configuration (SKIPPED - using existing)"
fi

# ════════════════════════════════════════════════════
# PHASE 4: Build & Deploy
# ════════════════════════════════════════════════════
step "Phase 4/5: Build & Deploy (${METHOD})"

cd "${PROJECT_ROOT}"

if [ "${METHOD}" = "docker" ]; then
    # ── Docker Deploy ────────────────────────────────
    log "Syncing project files to server..."
    rsync -az --delete \
        -e "ssh ${SSH_OPTS}" \
        --exclude '.git' \
        --exclude 'build/' \
        --exclude '.env' \
        --exclude 'node_modules/' \
        "${PROJECT_ROOT}/" "${SSH_USER}@${HOST}:${REMOTE_DIR}/src/"

    log "Copying Docker Compose config..."
    ${SCP_CMD} "${PROJECT_ROOT}/deploy/hostinger/docker-compose.production.yml" \
        "${SSH_USER}@${HOST}:${REMOTE_DIR}/docker-compose.yml"

    log "Building Docker image & starting container..."
    ${SSH_CMD} <<'DOCKER_DEPLOY'
set -e
cd /opt/picoclaw

echo "[REMOTE] Building Docker image (this may take a few minutes)..."
docker compose build picoclaw-gateway

echo "[REMOTE] Stopping existing container..."
docker compose down --timeout 30 2>/dev/null || true

echo "[REMOTE] Starting PicoClaw gateway..."
docker compose up -d picoclaw-gateway

echo "[REMOTE] Waiting for container to start..."
sleep 5

if docker compose ps picoclaw-gateway 2>/dev/null | grep -q "Up\|running"; then
    echo "[REMOTE] Container is running!"
    docker compose ps
else
    echo "[REMOTE] Container status:"
    docker compose ps
    echo ""
    echo "[REMOTE] Container logs:"
    docker compose logs --tail=30 picoclaw-gateway
fi

docker image prune -f >/dev/null 2>&1
echo "[REMOTE] Docker deploy complete"
DOCKER_DEPLOY

else
    # ── Binary Deploy ────────────────────────────────
    log "Syncing source code to server..."
    rsync -az --delete \
        -e "ssh ${SSH_OPTS}" \
        --exclude '.git' \
        --exclude 'build/' \
        --exclude '.env' \
        "${PROJECT_ROOT}/" "${SSH_USER}@${HOST}:${REMOTE_DIR}/src/"

    log "Building and installing binary on server..."
    ${SSH_CMD} <<'BINARY_DEPLOY'
set -e
cd /opt/picoclaw/src
export PATH=$PATH:/usr/local/go/bin

echo "[REMOTE] Building PicoClaw..."
make build

# Backup current binary
if [ -f /opt/picoclaw/bin/picoclaw ]; then
    cp /opt/picoclaw/bin/picoclaw "/opt/picoclaw/backups/picoclaw-$(date +%Y%m%d%H%M%S).bak"
fi

cp build/picoclaw /opt/picoclaw/bin/picoclaw
chmod +x /opt/picoclaw/bin/picoclaw

echo "[REMOTE] Restarting picoclaw service..."
systemctl restart picoclaw

sleep 3
if systemctl is-active --quiet picoclaw; then
    echo "[REMOTE] Service is running"
    systemctl status picoclaw --no-pager -l
else
    echo "[REMOTE] Service status:"
    systemctl status picoclaw --no-pager -l || true
    echo "[REMOTE] Recent logs:"
    journalctl -u picoclaw --no-pager -n 20 || true
fi

echo "[REMOTE] Binary deploy complete"
BINARY_DEPLOY
fi

log "Build & deploy done"

# ════════════════════════════════════════════════════
# PHASE 5: Verification
# ════════════════════════════════════════════════════
step "Phase 5/5: Verification"

log "Running health checks..."

HEALTH_OK=false
for i in 1 2 3 4 5; do
    sleep 3
    if ${SSH_CMD} "curl -sf http://localhost:18790/health" &>/dev/null; then
        HEALTH_OK=true
        break
    fi
    info "Waiting for gateway to start... (attempt ${i}/5)"
done

# Get final status
${SSH_CMD} <<'VERIFY_SCRIPT'
echo ""
echo "── Server ────────────────────────────────────"
printf "  %-12s %s\n" "Hostname:" "$(hostname)"
printf "  %-12s %s\n" "Memory:" "$(free -h | awk '/Mem:/ {print $3 "/" $2}')"
printf "  %-12s %s\n" "Disk:" "$(df -h / | awk 'NR==2 {print $3 "/" $2 " (" $5 ")"}')"
echo ""

echo "── PicoClaw ──────────────────────────────────"
if docker compose -f /opt/picoclaw/docker-compose.yml ps 2>/dev/null | grep -q picoclaw; then
    printf "  %-12s %s\n" "Method:" "Docker"
    STATUS=$(docker compose -f /opt/picoclaw/docker-compose.yml ps --format '{{.Status}}' picoclaw-gateway 2>/dev/null)
    printf "  %-12s %s\n" "Container:" "${STATUS}"
elif systemctl is-active --quiet picoclaw 2>/dev/null; then
    printf "  %-12s %s\n" "Method:" "Binary (systemd)"
    printf "  %-12s %s\n" "Status:" "running"
else
    printf "  %-12s %s\n" "Status:" "unknown"
fi

echo ""
echo "── Health Check ──────────────────────────────"
if curl -sf http://localhost:18790/health >/dev/null 2>&1; then
    printf "  %-12s %s\n" "Gateway:" "HEALTHY (port 18790)"
else
    printf "  %-12s %s\n" "Gateway:" "not responding yet (may still be starting)"
fi

echo ""
echo "── Firewall ──────────────────────────────────"
if command -v ufw &>/dev/null; then
    ufw status | grep -E "^(Status|18790|22)" 2>/dev/null || true
fi
VERIFY_SCRIPT

# ════════════════════════════════════════════════════
# Summary
# ════════════════════════════════════════════════════
DEPLOY_END=$(date +%s)
DEPLOY_DURATION=$((DEPLOY_END - DEPLOY_START))

echo ""
echo -e "${CYAN}${BOLD}"
echo "  ╔═══════════════════════════════════════════╗"
if [ "${HEALTH_OK}" = true ]; then
echo "  ║       Deploy completed successfully!      ║"
else
echo "  ║     Deploy finished (check status)        ║"
fi
echo "  ╚═══════════════════════════════════════════╝"
echo -e "${NC}"
echo "  Host:       ${HOST}"
echo "  Method:     ${METHOD}"
echo "  Duration:   ${DEPLOY_DURATION}s"
echo "  Gateway:    http://${HOST}:18790"
if [ "${HEALTH_OK}" = true ]; then
echo -e "  Health:     ${GREEN}HEALTHY${NC}"
else
echo -e "  Health:     ${YELLOW}Starting up (check in a few seconds)${NC}"
fi
echo ""
echo "  Quick commands:"
echo "    Status:    ssh ${SSH_USER}@${HOST} 'docker compose -f /opt/picoclaw/docker-compose.yml ps'"
echo "    Logs:      ssh ${SSH_USER}@${HOST} 'docker compose -f /opt/picoclaw/docker-compose.yml logs -f'"
echo "    Health:    curl http://${HOST}:18790/health"
echo "    Redeploy:  $0 -h ${HOST} --skip-setup --skip-config"
echo ""
