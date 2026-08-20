package main

import (
	"context"
	"fmt"
	"github.com/ItsLukV/guild-tracker/internal/market"
	"github.com/ItsLukV/guild-tracker/internal/store"
	"github.com/ItsLukV/guild-tracker/internal/utils"
	"github.com/bwmarrin/discordgo"
	"gorm.io/gorm"
	"log"
	"sort"
	"time"
)

var marketCache = market.NewCache()

var commands = []*discordgo.ApplicationCommand{
	{
		Name:        "status",
		Description: "Shows when the fetcher last ran",
	},
	{
		Name:        "loss",
		Description: "Check a players loss",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "player",
				Description: "Player Username",
				Required:    true,
			},
		},
	},
	{
		Name:        "items",
		Description: "Check the market value of a players chest items",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "player",
				Description: "Player Username",
				Required:    true,
			},
		},
	},
	{
		Name:        "leaderboard",
		Description: "Check the leaderboard loss",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "leaderboard",
				Description: "The leaderboard to view",
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "Coins Spent", Value: "Coins Spent"},
					{Name: "Chest Profit", Value: "Chest Profit"},
				},
			},
		},
	},
	{
		Name:        "inactivity",
		Description: "Finds inactivity in the guild",
		// Options:     []*discordgo.ApplicationCommandOption{},
	},
}

var handlers = map[string]func(db *gorm.DB, s *discordgo.Session, i *discordgo.InteractionCreate){
	"loss":        loss,
	"items":       items,
	"leaderboard": leaderboard,
	"inactivity":  inactivity,
	"status":      status,
}

func status(db *gorm.DB, s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	var runs []store.FetcherRun
	db.Find(&runs)

	var fields []*discordgo.MessageEmbedField
	for _, r := range runs {
		status := "✅ success"
		if !r.Success {
			status = "❌ failed"
		}
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:  string(r.Mode),
			Value: fmt.Sprintf("<t:%d:R> — %s", r.RanAt.Unix(), status),
		})
	}

	embed := &discordgo.MessageEmbed{
		Title:  "Fetcher Status",
		Color:  0x3498db,
		Fields: fields,
	}
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}

func leaderboard(db *gorm.DB, s *discordgo.Session, i *discordgo.InteractionCreate) {
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
	switch m["leaderboard"].StringValue() {
	case "Coins Spent":
		{
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

			if err := pg.EditWithPages(s, i, pages); err != nil {
				log.Println("leaderboard: paginate error:", err)
			}
		}
	case "Chest Profit":
		{
			var chests []store.DungeonChest
			db.Joins("JOIN players ON players.minecraft_uuid = dungeon_chests.player_uuid").
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
				profit   int
			}
			totals := make(map[string]*playerProfit)
			for _, chest := range chests {
				marketValue := 0.0
				for _, reward := range chest.Rewards {
					itemID, qty := market.ParseReward(reward)
					if price, ok := marketCache.Price(itemID); ok {
						marketValue += price * float64(qty)
					}
				}

				p, ok := totals[chest.PlayerUUID]
				if !ok {
					name := usernames[chest.PlayerUUID]
					if name == "" {
						name = chest.PlayerUUID
					}
					p = &playerProfit{username: name}
					totals[chest.PlayerUUID] = p
				}
				p.profit += int(marketValue) - chest.Price
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
					r := results[idx]
					fields = append(fields, &discordgo.MessageEmbedField{
						Name:  fmt.Sprintf("#%d - %s", idx+1, r.username),
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

			if err := pg.EditWithPages(s, i, pages); err != nil {
				log.Println("leaderboard: paginate error:", err)
			}
		}
	}
}

func loss(db *gorm.DB, s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	type Result struct {
		DungeonType string
		DungeonTier int
		Total       int
	}

	opts := i.ApplicationCommandData().Options
	m := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(opts))
	for _, opt := range opts {
		m[opt.Name] = opt
	}

	opt, ok := m["player"]
	if !ok {
		msg := "Failed to load command parameter"
		sendFailedEmbed(msg, s, i)
		return
	}

	mojang, err := utils.UsernameToUUID(context.Background(), opt.StringValue())
	if err != nil || mojang == nil {
		msg := fmt.Sprintf("Found no data for %s", opt.StringValue())
		sendFailedEmbed(msg, s, i)
		log.Printf("Failed converting username to uuid\n%s", err)
		return
	}
	uuid := mojang.ID

	var results []Result
	db.Model(&store.DungeonChest{}).
		Select("dungeon_type, dungeon_tier, SUM(price) as total").
		Where("paid = ?", true).
		Where("player_uuid = ?", uuid).
		Group("dungeon_type, dungeon_tier").
		Scan(&results)

	var out []*discordgo.MessageEmbedField
	for _, r := range results {
		out = append(out, &discordgo.MessageEmbedField{
			Name:  fmt.Sprintf("%s %v", r.DungeonType, r.DungeonTier),
			Value: utils.ShortNumber(r.Total),
		})
	}

	displayName, err := utils.UUIDToName(uuid)
	if err != nil {
		log.Println("error:", err)
		msg := fmt.Sprintf("Found no minecraft account with the uuid: %s", uuid)
		sendFailedEmbed(msg, s, i)
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Chest Price Report",
		Description: "Loss earnings for " + displayName,
		Color:       0x2ecc71,
		Fields:      out,
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: utils.PlayerHeadURL(uuid, 128, true),
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Requested by " + i.Member.User.Username,
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
	if err != nil {
		log.Printf("failed to respond to interaction: %v", err)
	}

}

func items(db *gorm.DB, s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	opts := i.ApplicationCommandData().Options
	m := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(opts))
	for _, opt := range opts {
		m[opt.Name] = opt
	}

	opt, ok := m["player"]
	if !ok {
		msg := "Failed to load command parameter"
		sendFailedEmbed(msg, s, i)
		return
	}

	mojang, err := utils.UsernameToUUID(context.Background(), opt.StringValue())
	if err != nil || mojang == nil {
		msg := fmt.Sprintf("Found no data for %s", opt.StringValue())
		sendFailedEmbed(msg, s, i)
		log.Printf("Failed converting username to uuid\n%s", err)
		return
	}
	uuid := mojang.ID

	var chests []store.DungeonChest
	db.Where("paid = ? AND player_uuid = ?", true, uuid).Find(&chests)

	var runs int64
	db.Model(&store.DungeonChest{}).
		Where("player_uuid = ?", uuid).
		Distinct("run_id").
		Count(&runs)

	type itemStat struct {
		count int
		value int
	}
	byItem := make(map[string]*itemStat)
	var totalValue int
	for _, chest := range chests {
		for _, reward := range chest.Rewards {
			itemID, qty := market.ParseReward(reward)
			price, ok := marketCache.Price(itemID)
			if !ok {
				continue
			}
			value := int(price) * qty

			stat, ok := byItem[itemID]
			if !ok {
				stat = &itemStat{}
				byItem[itemID] = stat
			}
			stat.count++
			stat.value += value
			totalValue += value
		}
	}

	type itemEntry struct {
		item  string
		count int
		value int
	}
	entries := make([]itemEntry, 0, len(byItem))
	for item, stat := range byItem {
		entries = append(entries, itemEntry{item: item, count: stat.count, value: stat.value})
	}
	sort.Slice(entries, func(a, b int) bool {
		return entries[a].value > entries[b].value
	})

	const maxItems = 15
	if len(entries) > maxItems {
		entries = entries[:maxItems]
	}

	var out []*discordgo.MessageEmbedField
	for _, e := range entries {
		out = append(out, &discordgo.MessageEmbedField{
			Name:  e.item,
			Value: fmt.Sprintf("%s (x%d)", utils.ShortNumber(e.value), e.count),
		})
	}

	displayName, err := utils.UUIDToName(uuid)
	if err != nil {
		log.Println("error:", err)
		msg := fmt.Sprintf("Found no minecraft account with the uuid: %s", uuid)
		sendFailedEmbed(msg, s, i)
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Chest Items Report",
		Description: fmt.Sprintf("Item value for %s: %s\nRuns: %v\nWithout chest prices", displayName, utils.ShortNumber(totalValue), runs),
		Color:       0x1abc9c,
		Fields:      out,
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: utils.PlayerHeadURL(uuid, 128, true),
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Item prices from: https://eliteskyblock.com/",
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
	if err != nil {
		log.Printf("failed to respond to interaction: %v", err)
	}
}

func inactivity(db *gorm.DB, s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	var results []struct {
		PlayerUUID string
		Username   string
		Total      int
	}

	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	startOfNextMonth := startOfMonth.AddDate(0, 1, 0)

	err := db.Model(&store.Gexp{}).
		Select("gexps.player_uuid, players.username, SUM(gexps.gexp) AS total").
		Joins("JOIN players ON players.minecraft_uuid = gexps.player_uuid").
		Where("gexps.ts >= ? AND gexps.ts < ? AND players.in_guild = ?", startOfMonth, startOfNextMonth, true).
		Group("gexps.player_uuid, players.username").
		Order("total").
		Scan(&results).Error
	if err != nil {
		sendFailedEmbed("Failed to create the list", s, i)
		return
	}

	title := fmt.Sprintf("Inactivity - %s - %s",
		startOfMonth.Format("2006-01-02"), startOfNextMonth.Format("2006-01-02"))

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
			Title:  title,
			Color:  0x00ff00,
			Fields: fields,
			Footer: &discordgo.MessageEmbedFooter{
				Text: "Requested by " + i.Member.User.Username,
			},
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}

	if len(pages) == 0 {
		pages = append(pages, &discordgo.MessageEmbed{
			Title:       title,
			Description: "No activity data for this month.",
			Color:       0x00ff00,
		})
	}

	if err := pg.EditWithPages(s, i, pages); err != nil {
		log.Println("inactivity: paginate error:", err)
	}
}

func sendFailedEmbed(msg string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "Leaderboard - Coins Spent",
		Color:       0xfc0000,
		Description: msg,
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}
