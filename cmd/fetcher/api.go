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
				TierCompletions map[TierCompletions]float32 `json:"tier_completions"`
				Experience      float32                     `json:"experience"`
			} `json:"catacombs"`
			MasterCatacombs struct {
				TierCompletions map[TierCompletions]float32 `json:"tier_completions"`
			} `json:"master_catacombs"`
		} `json:"dungeon_types"`
		PlayerClasses map[PlayerClasses]struct {
			Experience float32 `json:"experience"`
		} `json:"player_classes"`
		Secrets int `json:"secrets"`
	} `json:"dungeons"`
}

type PlayerClasses = string

const (
	Healer  PlayerClasses = "healer"
	Mage    PlayerClasses = "mage"
	Berserk PlayerClasses = "berserk"
	Archer  PlayerClasses = "archer"
	Tank    PlayerClasses = "tank"
)

type TierCompletions string

const (
	total   TierCompletions = "total"
	Zero    TierCompletions = "0"
	First   TierCompletions = "1"
	Second  TierCompletions = "2"
	Third   TierCompletions = "3"
	Fourth  TierCompletions = "4"
	Fifth   TierCompletions = "5"
	Sixth   TierCompletions = "6"
	Seventh TierCompletions = "7"
)
