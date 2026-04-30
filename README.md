# AI 生图邀测 MVP

面向小范围邀测用户的中文 AI 文生图工具，包含邀请码注册、登录、生图任务、额度扣减、历史记录、本地图片保存和最小管理员后台。

## 使用规则

- 每次生成 1 张图片，创建任务时扣 1 点。
- 生成失败会自动退回 1 点，系统不自动重试；用户可调整提示词后重新提交。
- 生成图片和历史记录面向用户展示最近 30 天，请及时下载需要长期保存的图片。

## Local development

1. Copy `.env.example` to `.env`.
2. Fill `OPENAI_BASE_URL`, `OPENAI_API_KEY`, `SESSION_SECRET`, and admin credentials.
3. Run `docker compose up --build`.
4. Visit `http://localhost` for local Caddy or the configured domain in production.

## Production

Point `img.bfsmlt.top` to the VPS IP, copy `.env.example` to `.env`, set `DOMAIN=img.bfsmlt.top`, then run:

```bash
docker compose up -d --build
```

Caddy listens on ports `80` and `443`, serves the React web app, and proxies `/api/*` plus `/healthz` to the Go API. Postgres data, Caddy state, and generated images are stored in Docker volumes.

For the first production setup, create `.env` on the VPS and keep it out of git:

```bash
APP_BASE_URL=https://img.bfsmlt.top
DOMAIN=img.bfsmlt.top
ADMIN_USERNAME=<admin-user>
ADMIN_PASSWORD=<strong-admin-password>
DATABASE_URL=postgres://postgres:postgres@postgres:5432/imagecreate?sslmode=disable
SESSION_SECRET=<long-random-secret>
OPENAI_BASE_URL=<openai-compatible-base-url>
OPENAI_API_KEY=<openai-compatible-api-key>
OPENAI_IMAGE_MODEL=gpt-image-2
OPENAI_REQUEST_TIMEOUT_SECONDS=600
WORKER_CONCURRENCY=2
IMAGE_SIZE_PRESETS={"1:1":"1024x1024","3:4":"768x1024","4:3":"1024x768","9:16":"720x1280","16:9":"1280x720"}
IMAGE_STORAGE_DIR=/data/images
IMAGE_RETENTION_DAYS=30
```

`WORKER_CONCURRENCY` controls how many generation workers run inside one API container. Each worker processes one image task at a time, so `2` allows up to two simultaneous upstream image requests per API container. Start with `2` for small invite tests, raise carefully after watching upstream rate limits and average latency.

## GitHub Actions Deploy

The repository includes a `CI` workflow:

- `verify`: runs Go tests, web tests, the web production build, and `docker compose config`.
- `deploy`: after `verify` succeeds, packages the verified revision, copies it to the VPS over SSH, runs `docker compose up -d --build`, and checks `/healthz`.

Add these GitHub repository secrets before relying on automated deployment:

```text
VPS_HOST=154.64.230.197
VPS_USER=<ssh-user>
VPS_SSH_KEY=<private-key-with-access-to-the-vps>
VPS_APP_DIR=/opt/imageCreate
APP_HEALTHCHECK_URL=https://img.bfsmlt.top/healthz
```

Add this GitHub repository variable to enable automatic deploys after `master` passes CI:

```text
ENABLE_PRODUCTION_DEPLOY=true
```

Deployment can also be started manually from the GitHub Actions tab by running the `CI` workflow. The VPS must have Docker and Docker Compose installed. The `VPS_USER` account must be allowed to write to `VPS_APP_DIR` and run Docker commands.

## Verification

```bash
cd api && go test ./...
cd web && npm test -- --run && npm run build
docker compose config
docker compose build
```

## Smoke test notes

Without a local `.env`, Docker Compose uses HTTP-only local defaults so the stack can boot without trying to issue a certificate for the example production domain. For a full production smoke test, set `DOMAIN`, `APP_BASE_URL`, `OPENAI_BASE_URL`, `OPENAI_API_KEY`, `SESSION_SECRET`, and admin credentials in `.env`.

The local smoke path is:

1. Start the stack with `docker compose up -d --build`.
2. Log in with the configured admin account.
3. Create an invite with 5 credits.
4. Register a user with that invite and confirm the user starts with 5 credits.
5. Submit a generation prompt and wait for the task to leave `queued` or `running`.
6. Check the admin generation audit list. It should include metadata such as prompt, status, size, latency, and error fields, but not image URLs, image paths, or previews.

If the OpenAI-compatible upstream values are placeholders, the generation task is expected to fail with an upstream error and refund the credit. A successful image smoke test requires real upstream credentials.
