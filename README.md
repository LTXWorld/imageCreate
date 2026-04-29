# AI 生图邀测 MVP

面向小范围邀测用户的中文 AI 文生图工具，包含邀请码注册、登录、生图任务、额度扣减、历史记录、本地图片保存和最小管理员后台。

## Local development

1. Copy `.env.example` to `.env`.
2. Fill `OPENAI_BASE_URL`, `OPENAI_API_KEY`, `SESSION_SECRET`, and admin credentials.
3. Run `docker compose up --build`.
4. Visit `http://localhost` for local Caddy or the configured domain in production.

## Production

Point the selected subdomain to the VPS IP, copy `.env.example` to `.env`, set `DOMAIN`, then run:

```bash
docker compose up -d --build
```

Caddy listens on ports `80` and `443`, serves the React web app, and proxies `/api/*` plus `/healthz` to the Go API. Postgres data, Caddy state, and generated images are stored in Docker volumes.

## Verification

```bash
cd api && go test ./...
cd web && npm test -- --run && npm run build
docker compose config
docker compose build
```
