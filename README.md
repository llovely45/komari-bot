# Komari Telegram Bot

基于 Go + Telegram Bot API + SQLite 的 Komari 续费提醒机器人。

## 功能

- `/admin` 打开管理面板
- 添加服务器：显示 Komari 中尚未加入 bot 的节点，支持刷新后确认添加
- 延迟监测：查看已添加服务器最近几小时的 Ping 聚合数据
- 续费提醒：服务器在到期前 `5` 天内开始提醒，未点击“已续费”前每天提醒一次
- 已续费按钮：点击后停止当前到期周期的提醒，直到 Komari 中该服务器出现下一次新的到期日

## 使用的 Komari 接口

- `GET /api/nodes`
- `GET /api/records/ping`

Komari API 文档：

- [API 接口](https://komari-document.pages.dev/dev/api)
- [GitHub 原始文档](https://raw.githubusercontent.com/komari-monitor/komari-document/main/dev/api.md)

## 环境变量

参考 [.env.example](/Users/lin8177/Documents/komari-bot/.env.example)。

- `TELEGRAM_BOT_TOKEN`：Telegram bot token
- `TELEGRAM_ADMIN_IDS`：允许使用 `/admin` 的 Telegram 用户 ID，逗号分隔
- `TELEGRAM_NOTIFY_CHAT_IDS`：接收续费提醒的 chat id，逗号分隔；不填则回退到 `TELEGRAM_ADMIN_IDS`
- `KOMARI_URL`：Komari 站点地址
- `KOMARI_KEY`：Komari API Key
- `DATABASE_PATH`：SQLite 数据库路径
- `TZ`：时区，默认 `Asia/Shanghai`
- `REMINDER_DAYS`：提前几天开始提醒，默认 `5`
- `PING_HOURS`：延迟数据查询范围，默认 `4`
- `CHECK_INTERVAL_MINUTES`：提醒检查间隔，默认 `60`
- `FX_API_URL`：汇率接口，默认 `https://api.frankfurter.app/latest`

## Docker 部署

1. 复制环境变量模板：

```bash
cp .env.example .env
```

2. 填写 `.env` 中的参数，至少包括：

```env
TELEGRAM_BOT_TOKEN=...
TELEGRAM_ADMIN_IDS=123456789
KOMARI_URL=https://komari.example.com
KOMARI_KEY=...
```

3. 启动：

```bash
docker compose up -d --build
```

## 直接拉取公开镜像

仓库发布后，可直接使用公开镜像：

```bash
docker pull ghcr.io/llovely45/komari-bot:latest
```

```yaml
services:
  komari-tg-bot:
    image: ghcr.io/llovely45/komari-bot:latest
    container_name: komari-tg-bot
    restart: unless-stopped
    env_file:
      - .env
    volumes:
      - ./data:/app/data
```

## 本地运行

```bash
go mod tidy
go run ./cmd/komari-bot
```

## 提醒逻辑

- 只针对已添加到 bot 的服务器
- `expired_at` 为空或未设置时不会提醒
- 到期前 `REMINDER_DAYS` 天内开始提醒
- 未点击“已续费”时，每天最多提醒一次
- 点击“已续费”后，当前 `expired_at` 对应周期不再提醒
- 当 Komari 中该服务器的到期日期发生变化时，提醒状态自动重置
