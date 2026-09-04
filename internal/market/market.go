package market

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	bazaarURL   = "https://api.eliteskyblock.com/resources/bazaar"
	auctionsURL = "https://api.eliteskyblock.com/resources/auctions"
)

type bazaarResponse struct {
	Products map[string]struct {
		// Sell      float64 `json:"sell"`
		SellOrder float64 `json:"sellOrder"`
	} `json:"products"`
}

type auctionsResponse struct {
	Items map[string][]struct {
		VariantKey string  `json:"variantKey"`
		Lowest     float64 `json:"lowest"`
		Last       float64 `json:"last"`
	} `json:"items"`
}

type Cache struct {
	client    *http.Client
	mu        sync.RWMutex
	lastFetch time.Time
	prices    map[string]float64
}

func NewCache() *Cache {
	return &Cache{
		client: &http.Client{Timeout: 30 * time.Second},
		prices: map[string]float64{},
	}
}

func (c *Cache) LastFetch() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastFetch
}

func (c *Cache) Price(itemID string) (float64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.prices[strings.ToUpper(itemID)]
	return p, ok
}

func ParseReward(reward string) (itemID string, quantity int) {
	parts := strings.Split(reward, ":")
	if len(parts) == 3 && strings.EqualFold(parts[0], "essence") {
		if qty, err := strconv.Atoi(parts[2]); err == nil {
			return strings.ToUpper(parts[0] + "_" + parts[1]), qty
		}
	}
	return strings.ToUpper(reward), 1
}

func (c *Cache) Refresh() error {
	prices := map[string]float64{}

	bazaar, err := c.fetchBazaar()
	if err != nil {
		return fmt.Errorf("fetch bazaar: %w", err)
	}
	for id, sell := range bazaar {
		prices[id] = sell
	}

	auctions, err := c.fetchAuctions()
	if err != nil {
		return fmt.Errorf("fetch auctions: %w", err)
	}
	for id, lowest := range auctions {
		if _, ok := prices[id]; !ok {
			prices[id] = lowest
		}
	}

	c.mu.Lock()
	c.prices = prices
	c.lastFetch = time.Now()
	c.mu.Unlock()

	return nil
}

func (c *Cache) fetchBazaar() (map[string]float64, error) {
	var out bazaarResponse
	if err := c.getJSON(bazaarURL, &out); err != nil {
		return nil, err
	}

	prices := make(map[string]float64, len(out.Products))
	for id, product := range out.Products {
		if product.SellOrder > 0 {
			prices[strings.ToUpper(id)] = product.SellOrder
		}
	}
	return prices, nil
}

func (c *Cache) fetchAuctions() (map[string]float64, error) {
	var out auctionsResponse
	if err := c.getJSON(auctionsURL, &out); err != nil {
		return nil, err
	}

	prices := make(map[string]float64, len(out.Items))
	for id, entries := range out.Items {
		for _, e := range entries {
			if e.VariantKey != "" {
				continue
			}
			price := e.Lowest
			if price <= 0 {
				price = e.Last
			}
			if price > 0 {
				prices[strings.ToUpper(id)] = price
			}
			break
		}
	}
	return prices, nil
}

func (c *Cache) getJSON(url string, out any) error {
	resp, err := c.client.Get(url)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Cache) StartAutoRefresh(interval time.Duration, logf func(format string, v ...any)) {
	if err := c.Refresh(); err != nil {
		logf("market: initial refresh failed: %v", err)
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			if err := c.Refresh(); err != nil {
				logf("market: refresh failed: %v", err)
			}
		}
	}()
}
