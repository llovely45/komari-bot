package currency

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Converter struct {
	apiURL string
	client *http.Client
	mu     sync.Mutex
	cache  map[string]cachedRate
}

type cachedRate struct {
	rate      float64
	expiresAt time.Time
}

type frankfurterResponse struct {
	Amount float64            `json:"amount"`
	Base   string             `json:"base"`
	Date   string             `json:"date"`
	Rates  map[string]float64 `json:"rates"`
}

func NewConverter(apiURL string) *Converter {
	return &Converter{
		apiURL: apiURL,
		client: &http.Client{Timeout: 10 * time.Second},
		cache:  map[string]cachedRate{},
	}
}

func (c *Converter) FormatCNY(price float64, currency string) string {
	switch {
	case price < 0:
		return "免费"
	case price == 0:
		return "未设置"
	}

	code := normalizeCurrency(currency)
	if code == "CNY" {
		return fmt.Sprintf("%.2f 人民币", price)
	}
	if code == "" {
		return fmt.Sprintf("原价 %s%.2f（币种无法识别）", strings.TrimSpace(currency), price)
	}

	rate, err := c.rateToCNY(code)
	if err != nil {
		return fmt.Sprintf("原价 %s%.2f（汇率获取失败）", formatOriginal(code, currency), price)
	}

	converted := price * rate
	return fmt.Sprintf("%.2f 人民币（原价 %s%.2f）", converted, formatOriginal(code, currency), price)
}

func (c *Converter) rateToCNY(code string) (float64, error) {
	if code == "CNY" {
		return 1, nil
	}

	c.mu.Lock()
	cached, ok := c.cache[code]
	if ok && time.Now().Before(cached.expiresAt) {
		c.mu.Unlock()
		return cached.rate, nil
	}
	c.mu.Unlock()

	endpoint, err := url.Parse(c.apiURL)
	if err != nil {
		return 0, err
	}
	query := endpoint.Query()
	query.Set("from", code)
	query.Set("to", "CNY")
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return 0, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("fx api error %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload frankfurterResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, err
	}

	rate, ok := payload.Rates["CNY"]
	if !ok || rate <= 0 {
		return 0, fmt.Errorf("missing CNY rate")
	}

	c.mu.Lock()
	c.cache[code] = cachedRate{
		rate:      rate,
		expiresAt: time.Now().Add(12 * time.Hour),
	}
	c.mu.Unlock()

	return rate, nil
}

func normalizeCurrency(raw string) string {
	value := strings.ToUpper(strings.TrimSpace(raw))
	switch value {
	case "", "-", "NONE":
		return ""
	case "¥", "￥", "CNY", "RMB":
		return "CNY"
	case "$", "USD", "US$", "US DOLLAR":
		return "USD"
	case "€", "EUR":
		return "EUR"
	case "£", "GBP":
		return "GBP"
	case "JPY", "JP¥", "YEN", "円":
		return "JPY"
	case "HKD", "HK$":
		return "HKD"
	case "TWD", "NT$":
		return "TWD"
	case "SGD", "S$":
		return "SGD"
	case "RUB", "₽":
		return "RUB"
	case "KRW", "₩":
		return "KRW"
	case "AUD", "A$":
		return "AUD"
	case "CAD", "C$":
		return "CAD"
	}

	if len(value) == 3 {
		return value
	}
	return ""
}

func formatOriginal(code, raw string) string {
	symbol := strings.TrimSpace(raw)
	if symbol == "" || strings.EqualFold(symbol, code) {
		return code + " "
	}
	return symbol
}
