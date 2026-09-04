package commands

import (
	"fmt"
	"time"

	"github.com/ItsLukV/guild-tracker/internal/store"
	"github.com/bwmarrin/discordgo"
	"gorm.io/gorm"
)

func (c *Commands) status(db *gorm.DB, s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	var status []store.FetcherRun
	db.Find(&status)

	var fields []*discordgo.MessageEmbedField
	for _, r := range status {
		status := "✅ success"
		if !r.Success {
			status = "❌ failed"
		}
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:  string(r.Mode),
			Value: fmt.Sprintf("<t:%d:R> — %s", r.RanAt.Unix(), status),
		})
	}

	fields = append(fields, &discordgo.MessageEmbedField{
		Name:  "Market Price",
		Value: fmt.Sprintf("<t:%d:R>", c.MarketCache.LastFetch().Unix()),
	})

	embed := &discordgo.MessageEmbed{
		Title:  "Fetcher Status",
		Color:  0x3498db,
		Fields: fields,
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Requested by " + i.Member.User.Username,
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}
