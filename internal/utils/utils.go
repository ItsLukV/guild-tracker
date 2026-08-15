package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type profileResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func UUIDToName(uuid string) (string, error) {
	// The sessionserver endpoint expects a UUID without dashes.
	uuid = strings.ReplaceAll(uuid, "-", "")

	url := "https://sessionserver.mojang.com/session/minecraft/profile/" + uuid

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// continue below
	case http.StatusNoContent, http.StatusNotFound:
		return "", fmt.Errorf("no profile found for uuid %q", uuid)
	default:
		return "", fmt.Errorf("unexpected status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading body: %w", err)
	}

	var profile profileResponse
	if err := json.Unmarshal(body, &profile); err != nil {
		return "", fmt.Errorf("parsing json: %w", err)
	}

	return profile.Name, nil
}

func PlayerHeadURL(uuid string, size int, overlay bool) string {
	uuid = strings.ReplaceAll(uuid, "-", "")

	url := fmt.Sprintf("https://crafatar.com/avatars/%s?size=%d", uuid, size)
	if overlay {
		url += "&overlay"
	}
	return url
}

func ShortNumber(n int) string {
	if n < 0 {
		return "-" + ShortNumber(-n)
	}
	if n < 1000 {
		return strconv.Itoa(n)
	}

	suffixes := []string{"K", "M", "B", "T", "Q"}

	// Determine which suffix bucket we fall into.
	exp := int(math.Log10(float64(n)) / 3)
	if exp > len(suffixes) {
		exp = len(suffixes)
	}

	value := float64(n) / math.Pow(1000, float64(exp))

	s := strconv.FormatFloat(value, 'f', 1, 64)
	s = strings.TrimSuffix(s, ".0")

	return s + suffixes[exp-1]
}
