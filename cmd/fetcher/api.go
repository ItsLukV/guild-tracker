package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"
)

const baseURL = "https://api.hypixel.net"

type Client struct {
	http   *http.Client
	apiKey string
}

func NewClient(apiKey string) *Client {
	if apiKey == "" {
		log.Panicln("No api key")
	}
	return &Client{
		http:   &http.Client{Timeout: 15 * time.Second},
		apiKey: apiKey,
	}
}

func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	url := baseURL + path
	if len(query) > 0 {
		url += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if c.apiKey != "" {
		req.Header.Set("API-Key", c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiErr struct {
			Cause string `json:"cause"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		if apiErr.Cause == "" {
			apiErr.Cause = resp.Status
		}
		return fmt.Errorf("hypixel API %d: %s", resp.StatusCode, apiErr.Cause)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode body: %w", err)
	}
	return nil
}

func fetchGuildUUIDs(ctx context.Context, client *Client, guildName string) ([]string, error) {
	var out struct {
		Success bool   `json:"success"`
		Cause   string `json:"cause"`
		Guild   *struct {
			Members []struct {
				UUID string `json:"uuid"`
			} `json:"members"`
		} `json:"guild"`
	}
	if err := client.get(ctx, "/v2/guild?name="+url.QueryEscape(guildName), nil, &out); err != nil {
		log.Printf("%s", err)
		return nil, err
	}

	if out.Guild == nil {
		return nil, fmt.Errorf("no guild found: %q", guildName)
	}

	uuids := make([]string, len(out.Guild.Members))
	for i, m := range out.Guild.Members {
		uuids[i] = m.UUID
	}
	return uuids, nil
}

type ProfilesResponse struct {
	Success  bool      `json:"success"`
	Profiles []Profile `json:"profiles"`
}

func (p *Profile) GameMode() string {
	switch p.RawGameMode {
	case "":
		return "normal"
	default:
		return p.RawGameMode
	}
}

type Profile struct {
	ProfileID   string                `json:"profile_id"`
	CuteName    string                `json:"cute_name"`
	Selected    bool                  `json:"selected"`
	RawGameMode string                `json:"game_mode"`
	Members     map[string]PlayerData `json:"members"`
}

type PlayerData struct {
	Dungeons struct {
		Treasures struct {
			Chests []struct {
				Type         string `json:"type"`
				RunId        string `json:"run_id"`
				ChestId      string `json:"Chest_id"`
				TreasureType string `json:"treasure_type"`
				Rewards      struct {
					Rewards                []string `json:"rewards"`
					RolledRngMeterRandomly bool     `json:"rolled_rng_meter_randomly"`
				} `json:"rewards"`
				Quality       int  `json:"quality"`
				ShinyEligible bool `json:"shiny_eligible"`
				Paid          bool `json:"paid"`
				Rerolls       int  `json:"rerolls"`
			} `json:"chests"`
			Runs []struct {
				Type         string `json:"type"`
				RunId        string `json:"run_id"`
				CompletionTs int    `json:"completion_ts"`
				DungeonType  string `json:"dungeon_type"`
				DungeonTier  int    `json:"dungeon_tier"`
				Participants []struct {
					PlayerUUID     string `json:"player_uuid"`
					DisplayName    string `json:"display_name"`
					ClassMilestone int    `json:"class_milestone"`
				} `json:"participants"`
			} `json:"runs"`
		} `json:"treasures"`
		DungeonTypes struct {
			Catacombs struct {
				TierCompletions map[string]float32 `json:"tier_completions"`
				Experience      float32            `json:"experience"`
			} `json:"catacombs"`
			MasterCatacombs struct {
				TierCompletions map[string]float32 `json:"tier_completions"`
			} `json:"master_catacombs"`
		} `json:"dungeon_types"`
		PlayerClasses map[string]struct {
			Experience float32 `json:"experience"`
		} `json:"player_classes"`
		Secrets int `json:"secrets"`
	} `json:"dungeons"`
}
