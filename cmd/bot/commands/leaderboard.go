package commands

import (
	"fmt"
	"time"

	"github.com/ItsLukV/guild-tracker/internal/store"
	"github.com/ItsLukV/guild-tracker/internal/utils"
	"github.com/bwmarrin/discordgo"
)

type LeaderboardType int64

const (
	TotalRuns LeaderboardType = iota
	ChestProfit
	// CoinsSpent Deprecated
)

func (c *Commands) leaderboard(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	opts := i.ApplicationCommandData().Options
	m := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(opts))
	for _, opt := range opts {
		m[opt.Name] = opt
	}

	timePeriod := store.Total
	if _, ok := m["leaderboard"]; !ok {
		return
	}
	if _, ok := m["duration"]; ok {
		timePeriod = store.Duration(m["leaderboard"].IntValue())
	}
	switch LeaderboardType(m["leaderboard"].IntValue()) {
	// case CoinsSpent: c.coinSpentLeaderboard(s, i) Deprecated
	case ChestProfit:
		c.chestProfitLeaderboard(s, i, timePeriod)
	case TotalRuns:
		c.totalRunsLeaderboard(s, i, timePeriod)
	default:
		c.sendFailedEmbed("unknown leaderboard type", s, i)
	}
}

func (c *Commands) totalRunsLeaderboard(s *discordgo.Session, i *discordgo.InteractionCreate, duration store.Duration) {
	runs, err := store.TotalRunsByPlayer(c.db, duration)

	if err != nil {
		c.logger.Errorf("error fetching dungeon runs: %v", err)
		c.sendFailedEmbed("error fetching dungeon runs", s, i)
		return
	}

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
				Value: utils.ShortNumber(int(r.Count)),
			})
		}
		pages = append(pages, &discordgo.MessageEmbed{
			Title:  "Leaderboard - Total Runs",
			Color:  0x4287f5,
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

func (c *Commands) chestProfitLeaderboard(s *discordgo.Session, i *discordgo.InteractionCreate, duration store.Duration) {
	results, err := store.TotalProfitByPlayer(c.db, c.MarketCache, duration)
	if err != nil {
		c.logger.Errorf("error fetching dungeon profits: %v", err)
		c.sendFailedEmbed("error fetching dungeon profits", s, i)
		return
	}

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

			c.db.Model(&store.DungeonChest{}).
				Joins("JOIN players on players.minecraft_uuid = dungeon_chests.player_uuid").
				Where("players.username = ?", r.Username).
				Distinct("run_id").
				Scopes(store.ApplyDuration(duration, "dungeon_chests.created_at")).
				Count(&runs)

			profitRate := utils.ShortNumber(r.Profit / int(runs))
			fields = append(fields, &discordgo.MessageEmbedField{
				Name:  fmt.Sprintf("#%d - %s (avg. %v/run)", idx+1, r.Username, profitRate),
				Value: utils.ShortNumber(r.Profit),
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

// Deprecated: not a fun stat to look
func (c *Commands) coinSpentLeaderboard(s *discordgo.Session, i *discordgo.InteractionCreate) {
	type Result struct {
		PlayerUUID string
		Username   string
		Total      int
	}
	var results []Result
	c.db.Model(&store.DungeonChest{}).
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
