FROM --platform=$BUILDPLATFORM golang:1.25 AS build

WORKDIR /app

ARG TARGETOS
ARG TARGETARCH

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/komari-bot ./cmd/komari-bot

FROM debian:bookworm-slim

LABEL org.opencontainers.image.title="komari-bot"
LABEL org.opencontainers.image.description="Telegram bot for Komari server expiry reminders and latency checks"
LABEL org.opencontainers.image.source="https://github.com/llovely45/komari-bot"
LABEL org.opencontainers.image.licenses="MIT"

ENV TZ=Asia/Shanghai

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=build /out/komari-bot /usr/local/bin/komari-bot

RUN mkdir -p /app/data

CMD ["komari-bot"]
