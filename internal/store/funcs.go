package store

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/ItsLukV/guild-tracker/internal/market"
	"gorm.io/gorm"
)

type PlayerRunCount struct {
	Username string
	Count    int64
}

func TotalRunsByPlayer(db *gorm.DB) ([]PlayerRunCount, error) {
	var runs []PlayerRunCount
	err := db.Model(&DungeonChest{}).
		Select("COUNT(DISTINCT dungeon_chests.run_id) as count, players.username as Username").
		Joins("JOIN players ON players.minecraft_uuid = dungeon_chests.player_uuid").
		Where("players.in_guild = ?", true).
		Group("players.username").
		Order("count DESC").
		Find(&runs).Error
	if err != nil {
		return nil, err
	}
	return runs, nil
}

type PlayerProfit struct {
	Username string
	Profit   int
}

func TotalProfitByPlayer(db *gorm.DB, cache *market.Cache) ([]PlayerProfit, error) {
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
