package commands

import (
	"fmt"
	"time"

	"github.com/ItsLukV/guild-tracker/internal/store"
	"github.com/ItsLukV/guild-tracker/internal/utils"
	"github.com/bwmarrin/discordgo"
	"gorm.io/gorm"
)

func (c *Commands) inactivity(db *gorm.DB, s *discordgo.Session, i *discordgo.InteractionCreate) {
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
		c.sendFailedEmbed("Failed to create the list", s, i)
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

	if err := c.pg.EditWithPages(s, i, pages); err != nil {
		c.logger.Errorf("inactivity: paginate error: %v", err)
	}
}
