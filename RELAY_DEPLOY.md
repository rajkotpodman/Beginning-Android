# Beginning Android — Relay Server Deployment Guide

The Beginning Android Relay Server is an ultra-lightweight, stateless, and blind WebSocket forwarder. It stores zero user data, maintains zero databases, and has zero knowledge of encryption keys or message content.

Under peak load, memory consumption is **< 15 MB RAM**, making it effortlessly runnable on the smallest $4–$5/mo VPS or free container tier.

---

## ⚡ Option A: $4–$5/mo VPS (Hetzner / DigitalOcean / Linode) + Caddy TLS (Recommended)

This setup provides automatic Let's Encrypt TLS certificates, auto-renewal, and high-performance WebSocket proxying.

### 1. Provision & SSH into your VPS
```bash
ssh root@<YOUR_SERVER_IP>
```

### 2. Install Docker & Caddy
```bash
# Ubuntu / Debian
apt update && apt install -y docker.io caddy git
systemctl enable --now docker caddy
```

### 3. Clone and Run the Relay Server
```bash
git clone https://github.com/rajkotpodman/Beginning-Android.git /opt/beginning-android
cd /opt/beginning-android/server

# Build and launch Docker container in background
docker build -t beginning-android-relay .
docker run -d \
  --name beginning-relay \
  --restart always \
  -p 127.0.0.1:8080:8080 \
  beginning-android-relay
```

### 4. Configure Caddy with Automatic Free TLS
Point your DNS (A record) `relay.yourdomain.com` to `<YOUR_SERVER_IP>`, then edit `/etc/caddy/Caddyfile`:

```caddyfile
relay.yourdomain.com {
    reverse_proxy 127.0.0.1:8080
}
```

Reload Caddy:
```bash
systemctl reload caddy
```

✅ **Your live secure relay is now running at:** `wss://relay.yourdomain.com/ws`

---

## 🚀 Option B: Railway.app (Zero-DevOps / Cloud Container)

1. Fork or push the repo to your GitHub account.
2. Log into [Railway.app](https://railway.app).
3. Click **New Project** $\to$ **Deploy from GitHub repo**.
4. Set the Root Directory to `/server`.
5. Under **Variables**, add:
   - `PORT`: `8080`
6. In **Settings** $\to$ **Networking**, click **Generate Domain** (e.g. `beginning-android-production.up.railway.app`).

✅ **Your live secure relay is:** `wss://beginning-android-production.up.railway.app/ws`

---

## 🪂 Option C: Fly.io (Global Edge)

```bash
cd server
fly launch --name beginning-android-relay --internal-port 8080 --no-deploy
fly deploy
```

Fly.io automatically handles SSL/TLS termination on `*.fly.dev`.

---

## 🔧 Configuring Clients to Use Your Live Relay

### Android Client
In `android/app/src/main/res/values/strings.xml`:
```xml
<string name="default_relay_url">wss://relay.yourdomain.com/ws</string>
```
Or directly enter your server URL on the pairing screen.

### Desktop Client (Windows & Linux)
In `~/.beginningandroid/config.json` (or `%USERPROFILE%\.beginningandroid\config.json` on Windows):
```json
{
  "relay_url": "wss://relay.yourdomain.com/ws"
}
```
Or update default in `desktop/internal/config/config.go`.

---

## 🧪 Verifying Server Health & WebSockets

Verify health endpoint via `curl`:
```bash
curl https://relay.yourdomain.com/health
# Response: {"status":"healthy","active_rooms":0,"version":"1.0.0"}
```

Run test suite against your live server:
```bash
TEST_SERVER_URL=wss://relay.yourdomain.com/ws python server/test_server.py
```
