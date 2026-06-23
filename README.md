# gdrive-bot

A production-grade private Telegram bot that manages your Google Drive:
upload files, browse folders, rename, delete, share, and generate links
— all from Telegram. Built in Go with MongoDB persistence, rclone for
optimised multi-threaded uploads, and an optional Redis cache.

---

## Features

| Feature | Detail |
|---|---|
| **Login** | Full Google OAuth2 loopback flow — one login, never again |
| **File explorer** | Real folder tree with breadcrumbs, pagination, search, refresh |
| **Upload** | Telegram files + direct HTTP/HTTPS links, progress bars, ETA |
| **Worker queue** | Configurable pool with retries, survives restarts |
| **File actions** | Rename, delete (with confirm), share toggle, view + direct link |
| **Owner commands** | Auth/unauth users, tune workers/queue/limits, stats, broadcast |
| **Auto-cleaner** | Periodically frees disk space from the downloads directory |
| **Cache** | Redis-backed (or in-memory fallback) for fast explorer navigation |

---

## Prerequisites

| Tool | Purpose |
|---|---|
| Go 1.22+ | Build the bot |
| MongoDB 6+ | All persistent state |
| rclone | Optimised Drive upload engine |
| Redis (optional) | Faster cache across restarts |

---

## Quick Start

### 1 — Google Cloud credentials

1. Open [Google Cloud Console](https://console.cloud.google.com/) → APIs & Services → Credentials.
2. Create an **OAuth 2.0 Client ID** → Application type: **Desktop app**.
3. Note the **Client ID** and **Client Secret**.
4. Enable the **Google Drive API** for the project.

### 2 — Telegram bot

1. Message [@BotFather](https://t.me/BotFather) → `/newbot`.
2. Note the **bot token**.
3. Message [@userinfobot](https://t.me/userinfobot) to get your **Telegram user ID** (this becomes `OWNER_ID`).

### 3 — Configure

```bash
git clone <repo> && cd gdrive-bot
make setup          # copies config/.env.example → config/.env
nano config/.env    # fill in all values
```

Minimum required variables:

```env
BOT_TOKEN=123456789:AAExampleToken
API_ID=123456
API_HASH=yourhash
OWNER_ID=111111111
GOOGLE_CLIENT_ID=your-client-id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=your-secret
MONGO_URI=mongodb://localhost:27017
```

### 4 — Install rclone

```bash
curl https://rclone.org/install.sh | bash
```

### 5 — Build and run

```bash
make tidy    # go mod tidy
make build   # produces build/gdrive-bot
./build/gdrive-bot
```

Or with Docker Compose (includes MongoDB + Redis):

```bash
docker compose up -d --build
```

### 6 — First use

Send `/login` to the bot and follow the two-step OAuth flow. After that:

- `/myfiles` — browse your Drive
- Send any file or `https://…` link — it's uploaded automatically

---

## Owner commands

| Command | Description |
|---|---|
| `/auth <id>` | Authorize a user |
| `/unauth <id>` | Revoke access |
| `/setworkers <n>` | Resize worker pool (live) |
| `/setqueue <n>` | Set queue capacity (restart to apply) |
| `/setdownloadlimit <MB\|0>` | Download size limit (0 = unlimited) |
| `/setuploadlimit <MB\|0>` | Upload size limit |
| `/stats` | CPU, RAM, disk, queue, uptime |
| `/disk` | Disk usage only |
| `/users` | List authorized users |
| `/broadcast <msg>` | Send to all authorized users |
| `/clean` | Clear the downloads directory |
| `/settings` | Show all current settings |
| `/restart` | Exit (supervisor restarts the process) |
| `/shutdown` | Stop the bot permanently |

---

## Systemd deployment (bare VPS)

```bash
# Create a dedicated user
useradd -r -s /sbin/nologin gdrive-bot

# Deploy
mkdir -p /opt/gdrive-bot
cp build/gdrive-bot /opt/gdrive-bot/
cp -r config /opt/gdrive-bot/
mkdir -p /opt/gdrive-bot/{downloads,logs}
chown -R gdrive-bot:gdrive-bot /opt/gdrive-bot

# Install and start
cp gdrive-bot.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now gdrive-bot

# View logs
journalctl -fu gdrive-bot
```

---

## Architecture

```
cmd/bot/main.go          — entrypoint, config load, signal handling
internal/
  bot/                   — composition root, queue.Processor, auto-cleaner
  handlers/              — all /command handlers (Deps bundle)
  callbacks/             — inline button routing
  explorer/              — file browser: breadcrumb, pagination, search
  drive/                 — Google Drive API v3 (list, rename, delete, share)
  auth/                  — Google OAuth2 flow
  rclone/                — rclone binary wrapper (optimised uploads)
  queue/                 — worker pool, retry, DB persistence
  download/              — Telegram file + direct-link downloaders
  upload/                — rclone upload orchestration
  progress/              — rate-limited Telegram message editor
  database/              — MongoDB repositories
  cache/                 — Redis / in-memory cache
  middleware/            — auth guard (cached)
  models/                — shared data types
  utils/                 — formatting, retry, disk, safe filenames
  config/                — static config from .env
```

---

## License

MIT
