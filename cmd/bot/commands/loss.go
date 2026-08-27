package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/ItsLukV/guild-tracker/internal/store"
	"github.com/ItsLukV/guild-tracker/internal/utils"
	"github.com/bwmarrin/discordgo"
	"gorm.io/gorm"
)

func (c *Commands) loss(db *gorm.DB, s *discordgo.Session, i *discordgo.InteractionCreate) {
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
		c.sendFailedEmbed(msg, s, i)
		return
	}

	mojang, err := utils.UsernameToUUID(context.Background(), opt.StringValue())
	if err != nil || mojang == nil {
		msg := fmt.Sprintf("Found no data for %s", opt.StringValue())
		c.sendFailedEmbed(msg, s, i)
		c.logger.Errorf("Failed converting username to uuid: %v", err)
		return
	}
	uuid := mojang.ID

	var results []struct {
		DungeonType string
		DungeonTier int
		Total       int
	}

	db.Model(&store.DungeonChest{}).
		Select("SUM(dungeon_chests.price) as total, dungeon_runs.dungeon_type, dungeon_runs.dungeon_tier").
		Joins("JOIN dungeon_runs ON dungeon_runs.run_id = dungeon_chests.run_id").
		Where("dungeon_chests.paid = ? AND dungeon_chests.player_uuid = ?", true, uuid).
		Group("dungeon_runs.dungeon_type, dungeon_runs.dungeon_tier").
		Find(&results)

	var out []*discordgo.MessageEmbedField
	for _, r := range results {
		out = append(out, &discordgo.MessageEmbedField{
			Name:  fmt.Sprintf("%s %v", r.DungeonType, r.DungeonTier),
			Value: utils.ShortNumber(r.Total),
		})
	}

	displayName, err := utils.UUIDToName(uuid)
	if err != nil {
		c.logger.Errorf("failed to resolve uuid to username: %v", err)
		msg := fmt.Sprintf("Found no minecraft account with the uuid: %s", uuid)
		c.sendFailedEmbed(msg, s, i)
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
		c.logger.Errorf("failed to respond to interaction: %v", err)
	}

}
