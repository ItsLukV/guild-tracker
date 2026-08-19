package main

import (
	"context"
	"fmt"
	"github.com/ItsLukV/guild-tracker/internal/store"
	"github.com/ItsLukV/guild-tracker/internal/utils"
	"github.com/bwmarrin/discordgo"
	"gorm.io/gorm"
	"log"
	"time"
)

var commands = []*discordgo.ApplicationCommand{
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
	"leaderboard": leaderboard,
	"inactivity":  inactivity,
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
	if err != nil {
		msg := fmt.Sprintf("Found no data for %s", opt.StringValue())
		sendFailedEmbed(msg, s, i)
		log.Println("Failed converting username to uuid\n%s", err)
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
