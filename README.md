# Komari Telegram Bot

基于 Go + Telegram Bot API + SQLite 的 Komari 续费提醒机器人。

## 功能

- `/admin` 打开管理面板
- 添加服务器：显示 Komari 中尚未加入 bot 的节点，支持分页浏览、刷新后确认添加
- 删除监听：显示已添加服务器，点击后立即取消监听并清除该服务器的提醒状态
- 延迟监测：查看已添加服务器最近几小时的 Ping 聚合数据，服务器列表分页展示
- 流量排行：按计费周期内总流量从高到低排序，显示上传、下载、总计
- 续费提醒：服务器在到期前 `10` 天内开始提醒，未点击“已续费”前每天 `0 点` 检查并提醒一次
- 已续费按钮：点击后停止当前到期周期的提醒，直到 Komari 中该服务器出现下一次新的到期日
- 管理面板消息：3 分钟无操作后自动删除
- 手动点击“立即检查提醒”时，提醒会直接发到当前操作的聊天窗口

## 使用的 Komari 接口

- `GET /api/nodes`
- `GET /api/records/ping`
- `GET /api/records/load`

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
- `REMINDER_DAYS`：提前几天开始提醒，默认 `10`
- `PING_HOURS`：延迟数据查询范围，默认 `4`
- `FX_API_URL`：汇率接口，默认 `https://api.frankfurter.app/latest`

如果你之前部署时 `.env` 里已经写了 `REMINDER_DAYS=5`，需要手动改成 `10` 后重启容器；否则运行中的实例仍然会按 `5` 天提醒。

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

### 一键下载 `.env` 并启动

先把下面 4 个必填参数换成你自己的值，再直接执行：

```bash
export TELEGRAM_BOT_TOKEN='123456:replace_me'
export TELEGRAM_ADMIN_IDS='123456789'
export KOMARI_URL='https://komari.example.com'
export KOMARI_KEY='replace_me'

bash <(curl -fsSL https://raw.githubusercontent.com/llovely45/komari-bot/main/scripts/docker-run.sh)
```

如果提醒要发到单独群组，可以额外加：

```bash
export TELEGRAM_NOTIFY_CHAT_IDS='-1001234567890'
```

脚本会自动完成这些动作：

- 下载 `.env.example` 并生成当前目录下的 `.env`
- 把你传入的环境变量写入 `.env`
- 拉取 `ghcr.io/llovely45/komari-bot:latest`
- 删除同名旧容器并重新 `docker run`

启动后查看日志：

```bash
docker logs -f komari-tg-bot
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
