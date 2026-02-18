# ⚡ PicoClaw Quick Reference

## 🚀 Setup (One-Time Only)

```bash
# 1. Secure VPS with Tailscale
make setup-tailscale

# 2. Configure Telegram Bot
make setup-telegram

# Done! 🎉
```

---

## 🔄 Daily Updates

```bash
# Sync with latest code
make sync-dev

# Check status
git status
```

---

## 🐛 Common Commands

```bash
# Build locally
make build

# Run locally (development)
make run

# Run tests
make test

# Check code quality
make check

# View logs on server
ssh root@YOUR_IP 'docker compose logs picoclaw | tail -50'

# Restart bot (if needed)
ssh root@YOUR_IP 'docker compose restart picoclaw'
```

---

## 📁 Important Files

```
picoclaw/
├── deploy/hostinger/
│   ├── setup-telegram.sh        ← Run: make setup-telegram
│   ├── setup-tailscale.sh       ← Run: make setup-tailscale
│   ├── setup-server.sh          ← Runs on VPS initial setup
│   └── docker-compose.production.yml
├── .github/workflows/
│   └── deploy-hostinger.yml     ← Auto-deploys on git push
├── config/
│   ├── config.json              ← Edit on VPS (nano)
│   └── .env                     ← Edit on VPS (nano)
└── docs/
    └── TELEGRAM_SETUP.md        ← Full guide with troubleshooting
```

---

## 🔐 Secrets Management

```bash
# Add/update GitHub Secret
gh secret set PICOCLAW_TELEGRAM_BOT_TOKEN -b "YOUR_TOKEN"

# List secrets (values hidden)
gh secret list

# Secrets used in deploy:
# - PICOCLAW_TELEGRAM_BOT_TOKEN
# - ANTHROPIC_API_KEY
# - HOSTINGER_HOST
# - HOSTINGER_SSH_USER
# - HOSTINGER_SSH_PASSWORD
# - HOSTINGER_SSH_PORT
```

---

## 📱 Telegram Bot

```bash
# Create bot: https://t.me/botfather
# Commands: /start, /help, /show, /list

# Get your Telegram user ID (check logs):
ssh root@YOUR_IP 'docker compose logs picoclaw | grep user_id'

# Add to whitelist (config/config.json):
"allow_from": ["123456789", "987654321"]
```

---

## 🔗 Tailscale

```bash
# Get your Tailnet IP
ssh root@YOUR_IP 'tailscale ip -4'

# Access via Tailnet
http://100.x.x.x:18790

# Or via hostname
https://picoclaw.YOUR-TAILNET.ts.net
```

---

## 🚨 If Something Breaks

```bash
# 1. Check logs
ssh root@YOUR_IP 'docker compose logs --tail=100 picoclaw'

# 2. Restart container
ssh root@YOUR_IP 'docker compose restart picoclaw'

# 3. Check if port is open
ssh root@YOUR_IP 'netstat -tuln | grep 18790'

# 4. Verify GitHub Secrets are set
gh secret list

# 5. Force redeploy
git commit --allow-empty -m "chore: trigger redeploy"
git push origin claude/hostinger-remote-deployment-TGVof
```

---

## 📚 Full Guides

- **Setup Complete Guide**: [SETUP_COMPLETE.md](SETUP_COMPLETE.md)
- **Sync & Git Guide**: [SYNC_GUIDE.md](SYNC_GUIDE.md)
- **Telegram Setup**: [docs/TELEGRAM_SETUP.md](docs/TELEGRAM_SETUP.md)
- **Telegram Quickstart**: [TELEGRAM_QUICKSTART.md](TELEGRAM_QUICKSTART.md)

---

## 💡 Tips

1. **Always `make sync-dev` before starting work**
2. **Use `make check` to verify code before pushing**
3. **GitHub Actions deploys automatically on push**
4. **Keep bot token in GitHub Secrets, never in code**
5. **Test locally with `make build && make run` first**

---

**Need help?** Check the full guides or open an issue! 🚀
