# AI 生图邀测 MVP 设计方案

## 背景与目标

本项目面向国内无法直接使用 ChatGPT/OpenAI 的小范围邀测用户，提供一个中文 AI 文生图工具。用户通过邀请码注册后，在网页中输入提示词并选择图片比例，由系统调用一组 OpenAI-compatible 反代接口生成图片。

MVP 目标是先稳定跑通完整闭环：邀请注册、登录、生图任务、额度扣减与失败退回、历史记录、本地图片保存、最小管理员后台和 Docker 单机部署。第一版不做公开商业化、在线支付、图片编辑、模板系统、提示词优化或社区图库。

## 范围

### MVP 包含

- 用户名、密码、邀请码注册。
- 登录、退出、当前用户会话。
- 中文文生图工作台。
- 提示词原样提交，不做自动优化或翻译。
- 用户选择图片比例/尺寸。
- 每次任务生成 1 张图。
- 每张图扣 1 次额度。
- 创建任务时先扣额度，失败自动退回。
- 每用户同时最多 1 个排队中或运行中的任务。
- 异步任务处理，前端轮询任务状态。
- 本地文件保存生成图片，默认保留 30 天。
- 图片通过后端鉴权读取，普通用户只能访问自己的图片。
- 用户可查看和删除自己的历史记录。
- 最小管理员后台：邀请码、用户、额度、禁用用户、审计记录。
- 管理员审计只看任务元数据，不直接查看图片。
- 通过子域名、HTTPS、Caddy、Docker Compose 部署到单台 VPS。

### MVP 不包含

- 在线支付或充值。
- 手机号、邮箱、短信验证、找回密码。
- 图片编辑、变体生成、参考图上传。
- 提示词优化按钮。
- 本地敏感词库维护。
- 公开作品流或社区互动。
- Redis 队列和独立 worker 服务。
- 多上游凭证轮询。

## 推荐方案

采用方案 1：单仓库 Docker Compose MVP，Go API + React/Vite 前端 + PostgreSQL + 本地文件存储 + Caddy。

该方案适合 2c2g 洛杉矶 VPS。Go 后端资源占用低，React/Vite 适合快速构建工具页和管理后台，PostgreSQL 承担用户、任务、额度和审计数据，Caddy 自动处理 HTTPS。worker 第一版作为 Go 服务内部 goroutine 运行，通过数据库领取任务。后续升级到 Redis + 独立 worker 时，可以保留现有任务表和上游 adapter，只替换任务调度方式。

## 运行架构

运行服务包括：

- `caddy`：绑定子域名，自动申请和续期 HTTPS 证书，转发前端静态资源和后端 API。
- `web`：React/Vite 前端，提供中文用户界面和管理员后台。
- `api`：Go 后端，提供 REST API、认证、权限、任务、额度、审计、图片鉴权访问。
- `postgres`：保存用户、邀请码、任务、额度流水和审计日志。
- `storage`：宿主机或 Docker volume 中的本地图片目录。

用户访问流程：

1. 用户访问 `https://img.example.com`。
2. Caddy 转发前端页面和 API 请求。
3. 用户注册或登录后创建生图任务。
4. Go API 校验余额、并发限制和输入参数。
5. Go API 写入任务并扣减 1 次额度。
6. 内部 worker 领取任务并调用上游反代接口。
7. 成功时保存图片并标记任务成功。
8. 失败时记录错误、标记任务失败并退回额度。
9. 前端轮询任务状态并展示结果或失败原因。

## 用户体验

第一版桌面端优先，手机端保证可用。

用户侧页面：

- 注册页：用户名、密码、邀请码。
- 登录页：用户名、密码。
- 生图工作台：提示词输入、比例选择、余额、生成按钮、当前任务状态、结果预览。
- 历史记录：展示 30 天内自己的任务，包含提示词、尺寸、状态、创建时间、图片结果和删除操作。

管理员侧模块：

- 用户管理：查看用户、余额、状态、创建时间，禁用或启用用户。
- 邀请码管理：创建邀请码，设置初始额度，查看使用状态。
- 额度管理：给用户增加或扣减额度，并写入额度流水。
- 审计记录：查看用户、提示词、任务状态、耗时、错误分类、额度变化和管理员操作。

管理员不通过审计页面查看用户生成图片。图片读取仍走用户归属鉴权。

## 认证与权限

认证使用用户名和密码。密码只保存哈希，不保存明文。

首个管理员通过环境变量初始化，例如：

- `ADMIN_USERNAME`
- `ADMIN_PASSWORD`

角色分为：

- `user`：普通邀测用户。
- `admin`：管理员。

权限规则：

- 未登录用户只能访问注册、登录和基础健康检查。
- 普通用户只能访问自己的任务、历史记录和图片。
- 管理员可以管理用户、邀请码、额度和审计记录。
- 管理员审计接口不返回图片二进制或图片访问链接。
- 禁用用户不能登录或创建任务。

## 数据模型

### `users`

保存用户身份、角色、状态和余额。

关键字段：

- `id`
- `username`
- `password_hash`
- `role`
- `status`
- `credit_balance`
- `created_at`
- `updated_at`

### `invites`

保存邀请码和使用状态。

关键字段：

- `id`
- `code`
- `initial_credits`
- `status`
- `created_by`
- `used_by`
- `used_at`
- `created_at`

邀请码只能使用一次。注册成功后创建用户，并写入一条注册赠送额度流水。

### `generation_tasks`

保存生图任务。

关键字段：

- `id`
- `user_id`
- `prompt`
- `size`
- `status`
- `upstream_model`
- `upstream_request_id`
- `image_path`
- `error_code`
- `error_message`
- `latency_ms`
- `created_at`
- `started_at`
- `completed_at`
- `deleted_at`

任务状态：

- `queued`
- `running`
- `succeeded`
- `failed`
- `canceled`

每个用户同一时间最多有 1 个 `queued` 或 `running` 任务。

### `credit_ledger`

保存额度流水，作为余额变化的审计来源。

关键字段：

- `id`
- `user_id`
- `task_id`
- `type`
- `amount`
- `balance_after`
- `reason`
- `actor_user_id`
- `created_at`

流水类型：

- `invite_grant`
- `admin_adjustment`
- `generation_debit`
- `generation_refund`

### `audit_logs`

保存管理员操作和关键系统事件。

关键字段：

- `id`
- `actor_user_id`
- `target_user_id`
- `action`
- `metadata`
- `created_at`

审计日志不保存 API Key、完整上游 URL 或其他敏感凭证。

## 生图参数

第一版只开放提示词和比例/尺寸。

支持比例：

- `1:1`
- `3:4`
- `4:3`
- `9:16`
- `16:9`

内部将比例映射为上游 `size` 字符串。默认映射如下，并允许通过环境变量覆盖，以适配反代接口实际支持范围：

- `1:1` -> `1024x1024`
- `3:4` -> `768x1024`
- `4:3` -> `1024x768`
- `9:16` -> `720x1280`
- `16:9` -> `1280x720`

固定请求参数：

- `model`: `gpt-image-2`
- `n`: `1`
- `quality`: `auto`
- `output_format`: `png`
- `background`: `auto`

## 上游适配器

上游接口按 OpenAI-compatible Images API 处理。

支持入口：

- `POST /v1/images/generations`

MVP 不使用：

- `POST /v1/images/edits`
- `POST /v1/images/variations`
- `POST /v1/responses`

配置项：

- `OPENAI_BASE_URL`
- `OPENAI_API_KEY`
- `OPENAI_IMAGE_MODEL`
- `OPENAI_REQUEST_TIMEOUT_SECONDS`

后端封装 `GenerateImage(ctx, prompt, size)`，前端不直接接触上游 URL、Key 或模型配置。

请求示例：

```json
{
  "model": "gpt-image-2",
  "prompt": "一张真实感照片...",
  "n": 1,
  "size": "1024x1024",
  "quality": "auto",
  "output_format": "png",
  "background": "auto"
}
```

adapter 将上游响应统一转成内部结果，包括图片数据、上游请求 ID、耗时和错误分类。后续如果反代响应格式变化，只改 adapter。

## 任务与额度流程

创建任务：

1. 用户提交提示词和尺寸。
2. 后端检查用户状态。
3. 后端检查提示词非空、长度上限和尺寸合法。
4. 后端检查余额至少为 1。
5. 后端检查该用户没有 `queued` 或 `running` 任务。
6. 后端开启数据库事务。
7. 后端扣减 1 次额度并写入 `generation_debit` 流水。
8. 后端创建 `queued` 任务。
9. 前端开始轮询任务状态。

执行任务：

1. worker 从数据库领取 `queued` 任务。
2. worker 将任务标记为 `running`。
3. worker 调用上游 `/v1/images/generations`。
4. 成功时保存 PNG 文件，写入 `image_path`，标记 `succeeded`。
5. 失败时标记 `failed`，写入错误分类和脱敏错误摘要。
6. 失败时退回 1 次额度并写入 `generation_refund` 流水。

删除历史：

- 用户删除自己的历史记录时，任务软删除。
- 对应图片可以立即删除，也可以由清理任务统一删除。
- 默认保留 30 天，清理任务删除过期图片和已软删除图片。

## 错误处理与内容安全

MVP 不维护本地敏感词库。内容安全主要依赖上游 AI 服务判断。

本地只做基础输入限制：

- 提示词不能为空。
- 提示词有长度上限。
- 尺寸必须来自允许列表。
- 用户必须有余额。
- 用户不能同时运行多个任务。

上游错误不原样透传给前端。后端将错误归类为：

- `content_rejected`：上游判断内容不支持生成。
- `rate_limited`：上游限流或服务繁忙。
- `timeout`：请求超时。
- `upstream_error`：其他上游错误。
- `internal_error`：本地保存、数据库或未知内部错误。

用户可见文案：

- 内容拒绝：`提示词可能包含不支持生成的内容，请调整描述后重试。`
- 限流：`当前生成服务繁忙，请稍后再试。`
- 超时：`生成超时，本次额度已退回，请稍后重试。`
- 其他失败：`生成失败，本次额度已退回。`

管理员审计中可以看到错误分类和截断后的上游错误摘要。日志和数据库不得保存 API Key、完整授权头或敏感凭证。

## 图片存储与访问

图片第一版保存在服务器本地目录或 Docker volume 中。

建议路径结构：

```text
storage/images/YYYY/MM/DD/<task-id>.png
```

图片访问只通过后端接口：

```text
GET /api/generations/:id/image
```

后端检查：

- 用户已登录。
- 任务存在。
- 任务属于当前用户。
- 任务状态为 `succeeded`。
- 图片文件存在。

管理员审计接口不返回图片 URL。未来迁移对象存储时，`image_path` 可以切换为对象 key，读取接口保持不变。

## API 设计

### Auth

- `POST /api/auth/register`
- `POST /api/auth/login`
- `POST /api/auth/logout`
- `GET /api/auth/me`

### User Generations

- `POST /api/generations`
- `GET /api/generations`
- `GET /api/generations/:id`
- `DELETE /api/generations/:id`
- `GET /api/generations/:id/image`

### Admin

- `GET /api/admin/users`
- `PATCH /api/admin/users/:id/status`
- `POST /api/admin/users/:id/credits`
- `GET /api/admin/invites`
- `POST /api/admin/invites`
- `GET /api/admin/audit-logs`
- `GET /api/admin/generation-tasks`

## 部署

部署目标是一台洛杉矶 VPS，配置约 2c2g。

使用 Docker Compose 编排：

- `caddy`
- `web`
- `api`
- `postgres`

域名使用子域名，例如 `img.example.com`，实际值通过环境变量配置。

关键环境变量：

- `APP_BASE_URL`
- `ADMIN_USERNAME`
- `ADMIN_PASSWORD`
- `DATABASE_URL`
- `SESSION_SECRET`
- `OPENAI_BASE_URL`
- `OPENAI_API_KEY`
- `OPENAI_IMAGE_MODEL`
- `OPENAI_REQUEST_TIMEOUT_SECONDS`
- `IMAGE_SIZE_PRESETS`
- `IMAGE_STORAGE_DIR`
- `IMAGE_RETENTION_DAYS`

第一版不引入 Kubernetes、Redis、消息队列或对象存储。

## 测试策略

后端测试重点：

- 邀请码只能使用一次。
- 注册成功写入初始额度流水。
- 密码不会明文保存。
- 禁用用户不能登录或创建任务。
- 余额不足不能创建任务。
- 创建任务会扣 1 次额度。
- 上游成功时任务变为 `succeeded` 且保存图片。
- 上游失败时任务变为 `failed` 且退回额度。
- 内容拒绝错误会映射为 `content_rejected`。
- 超时错误会映射为 `timeout`。
- 每用户同时最多 1 个 `queued` 或 `running` 任务。
- 普通用户不能访问别人的任务或图片。
- 管理员审计接口不返回图片。
- 上游 adapter 不记录 API Key。

前端测试重点：

- 注册、登录、退出流程。
- 工作台余额和任务状态展示。
- 创建任务后轮询状态。
- 成功任务展示结果。
- 失败任务展示中文错误原因。
- 历史记录删除。
- 管理员创建邀请码、调整额度、禁用用户。

部署验证重点：

- Docker Compose 可从空数据库启动。
- 首个管理员可由环境变量初始化。
- Caddy HTTPS 正常。
- 前端能访问 API。
- 图片鉴权访问正常。
- 服务重启后未完成任务可恢复或重新失败处理。

## 后续演进

方案 2 的升级路径：

- 引入 Redis 作为任务队列。
- 将 worker 从 Go API 进程中拆成独立服务。
- API 创建任务后推送 Redis job。
- worker 消费 Redis job，并继续更新 PostgreSQL 任务状态。
- 保留现有前端、任务表、额度流水、图片接口和上游 adapter。

后续功能候选：

- 图片编辑 `/v1/images/edits`。
- 提示词优化。
- 对象存储。
- 统计后台。
- 手动充值记录或在线支付。
- 多上游凭证轮询。
- 更细粒度的尺寸/质量计费。
