package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	TelegramBotToken      string
	TelegramAdminIDs      []int64
	TelegramNotifyChatIDs []int64
	KomariURL             string
	KomariKey             string
	DatabasePath          string
	Timezone              string
	ReminderDays          int
	PingHours             int
	FXAPIURL              string
}

func Load() (Config, error) {
	cfg := Config{
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		KomariURL:        strings.TrimRight(os.Getenv("KOMARI_URL"), "/"),
		KomariKey:        os.Getenv("KOMARI_KEY"),
		DatabasePath:     defaultString(os.Getenv("DATABASE_PATH"), "./data/komari-bot.db"),
		Timezone:         defaultString(os.Getenv("TZ"), "Asia/Shanghai"),
		ReminderDays:     defaultInt(os.Getenv("REMINDER_DAYS"), 5),
		PingHours:        defaultInt(os.Getenv("PING_HOURS"), 4),
		FXAPIURL:         defaultString(os.Getenv("FX_API_URL"), "https://api.frankfurter.app/latest"),
	}

	var err error
	cfg.TelegramAdminIDs, err = parseInt64List(os.Getenv("TELEGRAM_ADMIN_IDS"))
	if err != nil {
		return Config{}, fmt.Errorf("parse TELEGRAM_ADMIN_IDS: %w", err)
	}

	cfg.TelegramNotifyChatIDs, err = parseInt64List(os.Getenv("TELEGRAM_NOTIFY_CHAT_IDS"))
	if err != nil {
		return Config{}, fmt.Errorf("parse TELEGRAM_NOTIFY_CHAT_IDS: %w", err)
	}

	if len(cfg.TelegramNotifyChatIDs) == 0 {
		cfg.TelegramNotifyChatIDs = append([]int64(nil), cfg.TelegramAdminIDs...)
	}

	if cfg.TelegramBotToken == "" {
		return Config{}, errors.New("TELEGRAM_BOT_TOKEN is required")
	}
	if cfg.KomariURL == "" {
		return Config{}, errors.New("KOMARI_URL is required")
	}
	if len(cfg.TelegramAdminIDs) == 0 {
		return Config{}, errors.New("TELEGRAM_ADMIN_IDS is required")
	}
	if len(cfg.TelegramNotifyChatIDs) == 0 {
		return Config{}, errors.New("at least one TELEGRAM_NOTIFY_CHAT_IDS or TELEGRAM_ADMIN_IDS is required")
	}
	if cfg.ReminderDays < 1 {
		return Config{}, errors.New("REMINDER_DAYS must be >= 1")
	}
	if cfg.PingHours < 1 {
		return Config{}, errors.New("PING_HOURS must be >= 1")
	}
	return cfg, nil
}

func parseInt64List(raw string) ([]int64, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	parts := strings.Split(raw, ",")
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultInt(value string, fallback int) int {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
