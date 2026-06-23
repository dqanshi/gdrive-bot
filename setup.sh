#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# gdrive-bot first-run setup script
# Run this ONCE on the VPS after cloning/unzipping the project.
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

BOLD='\033[1m'; RESET='\033[0m'; GREEN='\033[32m'; RED='\033[31m'
ok()  { echo -e "${GREEN}✓${RESET} $*"; }
err() { echo -e "${RED}✗${RESET} $*"; exit 1; }

echo -e "${BOLD}gdrive-bot setup${RESET}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# ── Check prerequisites ───────────────────────────────────────────────────
command -v go    >/dev/null 2>&1 || err "Go 1.22+ is required. Install from https://go.dev/dl/"
command -v rclone>/dev/null 2>&1 || err "rclone is required. Install: curl https://rclone.org/install.sh | bash"
command -v mongod>/dev/null 2>&1 || echo "  ⚠  mongod not found — make sure MongoDB is running before starting the bot."
ok "Prerequisites checked"

# ── Directories ───────────────────────────────────────────────────────────
mkdir -p config downloads logs
ok "Directories created"

# ── Env file ──────────────────────────────────────────────────────────────
if [ ! -f config/.env ]; then
    cp config/.env.example config/.env
    ok "Created config/.env — please fill in your credentials."
    echo "  Edit: nano config/.env"
else
    ok "config/.env already exists"
fi

# ── Go modules ───────────────────────────────────────────────────────────
echo "Downloading Go modules (requires internet)..."
go mod tidy
ok "Go modules ready"

# ── Build ─────────────────────────────────────────────────────────────────
echo "Building..."
mkdir -p build
go build -ldflags="-s -w" -o build/gdrive-bot ./cmd/bot
ok "Binary built → build/gdrive-bot"

echo ""
echo -e "${BOLD}Setup complete!${RESET}"
echo "Next steps:"
echo "  1. Edit config/.env with your credentials"
echo "  2. Start: ./build/gdrive-bot"
echo "  3. Message your bot /login to connect Google Drive"
