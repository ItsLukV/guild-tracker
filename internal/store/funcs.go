package store

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/ItsLukV/guild-tracker/internal/market"
	"gorm.io/gorm"
)

type Duration int

const (
	Day Duration = iota
	Week
	Month
	Year
	Total
)

func (d Duration) String() string {
	switch d {
	case Day:
		return "Daily"
	case Week:
		return "Weekly"
	case Month:
		return "Monthly"
	case Year:
		return "Yearly"
	case Total:
		return "Total"
	default:
		return fmt.Sprintf("Unknown duration type: %d", int(d))
	}
}

func ApplyDuration(duration Duration, tsColumn string) func(*gorm.DB) *gorm.DB {
	return func(query *gorm.DB) *gorm.DB {
		now := time.Now()
		var startDate, endDate time.Time

		switch duration {
		case Day:
			startDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			endDate = startDate.AddDate(0, 0, 1)
		case Week:
			weekday := int(now.Weekday())
			if weekday == 0 {
				weekday = 7
			}
			startDate = time.Date(now.Year(), now.Month(), now.Day()-(weekday-1), 0, 0, 0, 0, now.Location())
			endDate = startDate.AddDate(0, 0, 7)
		case Month:
			startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
			endDate = startDate.AddDate(0, 1, 0)
		case Year:
			startDate = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
			endDate = startDate.AddDate(1, 0, 0)
		case Total:
			return query
		}

		clause := fmt.Sprintf("%s >= ? AND %s < ?", tsColumn, tsColumn)
		return query.Where(clause, startDate, endDate)
	}
}

type PlayerRunCount struct {
	Username string
	Count    int64
}

func TotalRunsByPlayer(db *gorm.DB, duration Duration) ([]PlayerRunCount, error) {
	var runs []PlayerRunCount
	err := db.Model(&DungeonChest{}).
		Select("COUNT(DISTINCT dungeon_chests.run_id) as count, players.username as Username").
		Joins("JOIN players ON players.minecraft_uuid = dungeon_chests.player_uuid").
		Where("players.in_guild = ?", true).
		Group("players.username").
		Scopes(ApplyDuration(duration, "dungeon_chests.created_at")).
		Order("count DESC").
		Find(&runs).Error

	if err != nil {
		return nil, err
	}
	return runs, nil
}

func RemoveShinyNecron(itemId string) string {
	switch itemId {
	case "SHINY_NECRON_HANDLE":
		return "NECRON_HANDLE"
	case "SHINY_WITHER_HELMET":
		return "WITHER_HELMET"
	case "SHINY_WITHER_CHESTPLATE":
		return "WITHER_CHESTPLATE"
	case "SHINY_WITHER_LEGGINGS":
		return "WITHER_LEGGINGS"
	case "SHINY_WITHER_BOOTS":
		return "WITHER_BOOTS"
	default:
		return itemId
	}
}

type PlayerProfit struct {
	Username string
	Profit   int
}

func TotalProfitByPlayer(db *gorm.DB, cache *market.Cache, duration Duration) ([]PlayerProfit, error) {
	var chests []struct {
		DungeonChest
		DungeonType string
		DungeonTier int
	}
	err := db.Model(&DungeonChest{}).
		Select("dungeon_chests.*, dungeon_runs.dungeon_type, dungeon_runs.dungeon_tier").
		Joins("JOIN players ON players.minecraft_uuid = dungeon_chests.player_uuid").
		Joins("JOIN dungeon_runs ON dungeon_runs.run_id = dungeon_chests.run_id").
		Where("dungeon_chests.paid = ? AND players.in_guild = ?", true, true).
		Scopes(ApplyDuration(duration, "dungeon_chests.created_at")).
		Find(&chests).Error

	if err != nil {
		return nil, err
	}

	var guildPlayers []Player
	err = db.Where("in_guild = ?", true).Find(&guildPlayers).Error
	if err != nil {
		return nil, err
	}

	usernames := make(map[string]string, len(guildPlayers))
	for _, p := range guildPlayers {
		usernames[p.MinecraftUUID] = p.Username
	}

	totals := make(map[string]*PlayerProfit)
	for _, chest := range chests {
		profit := 0
		for _, reward := range chest.Rewards {
			itemID, qty := market.ParseReward(reward)
			itemID = RemoveShinyNecron(itemID)

			price, ok := cache.Price(itemID)
			if !ok {
				continue
			}
			itemPrice := market.ChestPriceItems[chest.TreasureType][strconv.Itoa(chest.DungeonTier)][reward]
			profit += (int(price) - itemPrice) * qty
		}

		p, ok := totals[chest.PlayerUUID]
		if !ok {
			name := usernames[chest.PlayerUUID]
			if name == "" {
				name = chest.PlayerUUID
			}
			p = &PlayerProfit{Username: name}
			totals[chest.PlayerUUID] = p
		}
		p.Profit += profit
	}

	kismetPrice, exist := cache.Price("KISMET_FEATHER")
	if !exist {
		return nil, fmt.Errorf("failed to fetch kismet feather price")
	}

	for uuid, playerInfo := range totals {
		var rerolls struct {
			Rerolls int
		}

		db.Model(&DungeonChest{}).
			Select("SUM(dungeon_chests.Rerolls) as rerolls").
			Where("dungeon_chests.player_uuid = ?", uuid).
			Scopes(ApplyDuration(duration, "dungeon_chests.created_at")).
			Find(&rerolls)

		playerInfo.Profit -= int(kismetPrice) * rerolls.Rerolls
	}

	results := make([]PlayerProfit, 0, len(totals))
	for _, p := range totals {
		results = append(results, *p)
	}
	sort.Slice(results, func(a, b int) bool {
		return results[a].Profit > results[b].Profit
	})
	return results, nil
}
