package commands

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/ItsLukV/guild-tracker/internal/market"
	"github.com/ItsLukV/guild-tracker/internal/store"
	"github.com/ItsLukV/guild-tracker/internal/utils"
	"github.com/bwmarrin/discordgo"
	"gorm.io/gorm"
)

type LeaderboardType int64

const (
	TotalRuns LeaderboardType = iota
	ChestProfit
	CoinsSpent
)

func (c *Commands) leaderboard(db *gorm.DB, s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	opts := i.ApplicationCommandData().Options
	m := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(opts))
	for _, opt := range opts {
		m[opt.Name] = opt
	}

	if _, ok := m["leaderboard"]; !ok {
		return
	}
	switch LeaderboardType(m["leaderboard"].IntValue()) {
	case CoinsSpent:
		c.coinSpentLeaderboard(db, s, i)
	case ChestProfit:
		c.chestProfitLeaderboard(db, s, i)
	case TotalRuns:
		c.totalRunsLeaderboard(db, s, i)
	default:
		c.sendFailedEmbed("unknown leaderboard type", s, i)
	}
}

func (c *Commands) totalRunsLeaderboard(db *gorm.DB, s *discordgo.Session, i *discordgo.InteractionCreate) {
	var runs []struct {
		Username string
		Uuid     string
		Count    int
	}
	db.Model(&store.DungeonRun{}).Select("players.username, players.minecraft_uuid as uuid, COUNT(DISTINCT dungeon_runs.run_id) as count").
		Joins("JOIN profiles ON dungeon_runs.profile_id = profiles.profile_id").
		Joins("JOIN players ON players.minecraft_uuid = profiles.player_uuid").
		Group("profiles.player_uuid").
		Where("players.in_guild = ?", true).
		Order("count DESC").
		Find(&runs)

	const perPage = 10
	var pages []*discordgo.MessageEmbed

	for start := 0; start < len(runs); start += perPage {
		end := start + perPage
		if end > len(runs) {
			end = len(runs)
		}

		var fields []*discordgo.MessageEmbedField
		for idx := start; idx < end; idx++ {
			r := runs[idx]
			name := r.Username
			if name == "" {
				name = r.Username
			}
			fields = append(fields, &discordgo.MessageEmbedField{
				Name:  fmt.Sprintf("#%d - %s", idx+1, name),
				Value: utils.ShortNumber(r.Count),
			})
		}
		pages = append(pages, &discordgo.MessageEmbed{
			Title:  "Leaderboard - Coins Spent",
			Color:  0xf1c40f,
			Fields: fields,
			Footer: &discordgo.MessageEmbedFooter{
				Text: "Requested by " + i.Member.User.Username,
			},
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}

	if err := c.pg.EditWithPages(s, i, pages); err != nil {
		c.logger.Errorf("leaderboard: paginate error: %v", err)
	}
}

func (c *Commands) chestProfitLeaderboard(db *gorm.DB, s *discordgo.Session, i *discordgo.InteractionCreate) {
	var chests []struct {
		store.DungeonChest
		DungeonType string
		DungeonTier int
	}
	db.Model(&store.DungeonChest{}).
		Select("dungeon_chests.*, dungeon_runs.dungeon_type, dungeon_runs.dungeon_tier").
		Joins("JOIN players ON players.minecraft_uuid = dungeon_chests.player_uuid").
		Joins("JOIN dungeon_runs ON dungeon_runs.run_id = dungeon_chests.run_id").
		Where("dungeon_chests.paid = ? AND players.in_guild = ?", true, true).
		Find(&chests)

	var guildPlayers []store.Player
	db.Where("in_guild = ?", true).Find(&guildPlayers)
	usernames := make(map[string]string, len(guildPlayers))
	for _, p := range guildPlayers {
		usernames[p.MinecraftUUID] = p.Username
	}

	type playerProfit struct {
		username string
		uuid     string
		profit   int
	}
	totals := make(map[string]*playerProfit)
	for _, chest := range chests {
		profit := 0
		for _, reward := range chest.Rewards {
			itemID, qty := market.ParseReward(reward)
			price, ok := c.MarketCache.Price(itemID)
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
			p = &playerProfit{username: name, uuid: chest.PlayerUUID}
			totals[chest.PlayerUUID] = p
		}
		p.profit += profit
	}

	results := make([]playerProfit, 0, len(totals))
	for _, p := range totals {
		results = append(results, *p)
	}
	sort.Slice(results, func(a, b int) bool {
		return results[a].profit > results[b].profit
	})

	const perPage = 10
	var pages []*discordgo.MessageEmbed
	for start := 0; start < len(results); start += perPage {
		end := start + perPage
		if end > len(results) {
			end = len(results)
		}

		var fields []*discordgo.MessageEmbedField
		for idx := start; idx < end; idx++ {
			var runs int64
			r := results[idx]

			db.Model(&store.DungeonChest{}).
				Where("player_uuid = ?", r.uuid).
				Distinct("run_id").
				Count(&runs)

			profitRate := utils.ShortNumber(r.profit / int(runs))
			fields = append(fields, &discordgo.MessageEmbedField{
				Name:  fmt.Sprintf("#%d - %s (avg. %v/run)", idx+1, r.username, profitRate),
				Value: utils.ShortNumber(r.profit),
			})
		}
		pages = append(pages, &discordgo.MessageEmbed{
			Title:  "Leaderboard - Chest Profit",
			Color:  0x1abc9c,
			Fields: fields,
			Footer: &discordgo.MessageEmbedFooter{
				Text: "Item prices from: https://eliteskyblock.com/",
			},
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}

	if err := c.pg.EditWithPages(s, i, pages); err != nil {
		c.logger.Errorf("leaderboard: paginate error: %v", err)
	}
}

func (c *Commands) coinSpentLeaderboard(db *gorm.DB, s *discordgo.Session, i *discordgo.InteractionCreate) {
	type Result struct {
		PlayerUUID string
		Username   string
		Total      int
	}
	var results []Result
	db.Model(&store.DungeonChest{}).
		Select("dungeon_chests.player_uuid, players.username, SUM(dungeon_chests.price) as total").
		Joins("JOIN players ON players.minecraft_uuid = dungeon_chests.player_uuid").
		Where("dungeon_chests.paid = ? AND players.in_guild = ?", true, true).
		Group("dungeon_chests.player_uuid, players.username").
		Order("total DESC").
		Scan(&results)

	const perPage = 10
	var pages []*discordgo.MessageEmbed

	for start := 0; start < len(results); start += perPage {
		end := start + perPage
		if end > len(results) {
			end = len(results)
		}

		var fields []*discordgo.MessageEmbedField
		for idx := start; idx < end; idx++ {
			r := results[idx]
			name := r.Username
			if name == "" {
				name = r.PlayerUUID
			}
			fields = append(fields, &discordgo.MessageEmbedField{
				Name:  fmt.Sprintf("#%d - %s", idx+1, name),
				Value: utils.ShortNumber(r.Total),
			})
		}
		pages = append(pages, &discordgo.MessageEmbed{
			Title:  "Leaderboard - Coins Spent",
			Color:  0xf1c40f,
			Fields: fields,
			Footer: &discordgo.MessageEmbedFooter{
				Text: "Requested by " + i.Member.User.Username,
			},
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}

	if err := c.pg.EditWithPages(s, i, pages); err != nil {
		c.logger.Errorf("leaderboard: paginate error: %v", err)
	}
}
