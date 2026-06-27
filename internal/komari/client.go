package komari

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type responseEnvelope[T any] struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type Node struct {
	UUID         string  `json:"uuid"`
	Name         string  `json:"name"`
	Group        string  `json:"group"`
	Price        float64 `json:"price"`
	BillingCycle int     `json:"billing_cycle"`
	AutoRenewal  bool    `json:"auto_renewal"`
	Currency     string  `json:"currency"`
	ExpiredAt    string  `json:"expired_at"`
}

type PingData struct {
	Count     int            `json:"count"`
	BasicInfo []PingNodeInfo `json:"basic_info"`
	Tasks     []PingTask     `json:"tasks"`
}

type PingNodeInfo struct {
	Client string  `json:"client"`
	Loss   float64 `json:"loss"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
}

type PingTask struct {
	ID        int      `json:"id"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Interval  int      `json:"interval"`
	Loss      float64  `json:"loss"`
	Min       float64  `json:"min"`
	Max       float64  `json:"max"`
	Avg       float64  `json:"avg"`
	Total     int      `json:"total"`
	Clients   []string `json:"clients"`
	DefaultOn bool     `json:"default_on"`
}

type LoadData struct {
	Count   int          `json:"count"`
	Records []LoadRecord `json:"records"`
}

type LoadRecord struct {
	Client       string  `json:"client"`
	Time         string  `json:"time"`
	NetIn        float64 `json:"net_in"`
	NetOut       float64 `json:"net_out"`
	NetTotalUp   float64 `json:"net_total_up"`
	NetTotalDown float64 `json:"net_total_down"`
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (c *Client) FetchNodes() ([]Node, error) {
	var envelope responseEnvelope[[]Node]
	if err := c.getJSON("/api/nodes", nil, &envelope); err != nil {
		return nil, err
	}
	return envelope.Data, nil
}

func (c *Client) FetchPingData(uuid string, hours int) (PingData, error) {
	var envelope responseEnvelope[PingData]
	params := url.Values{}
	params.Set("uuid", uuid)
	params.Set("hours", fmt.Sprintf("%d", hours))
	if err := c.getJSON("/api/records/ping", params, &envelope); err != nil {
		return PingData{}, err
	}
	return envelope.Data, nil
}

func (c *Client) FetchLoadData(uuid string, hours int, loadType string) (LoadData, error) {
	var envelope responseEnvelope[LoadData]
	params := url.Values{}
	params.Set("uuid", uuid)
	params.Set("hours", fmt.Sprintf("%d", hours))
	if strings.TrimSpace(loadType) != "" {
		params.Set("load_type", loadType)
	}
	if err := c.getJSON("/api/records/load", params, &envelope); err != nil {
		return LoadData{}, err
	}
	return envelope.Data, nil
}

func (c *Client) getJSON(path string, params url.Values, target any) error {
	endpoint := c.baseURL + path
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("komari api error %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode komari response: %w", err)
	}

	return nil
}
