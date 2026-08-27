package commands

import (
	"context"
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

func (c *Commands) items(db *gorm.DB, s *discordgo.Session, i *discordgo.InteractionCreate) {
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

	var chests []struct {
		store.DungeonChest
		DungeonType string
		DungeonTier int
	}

	db.Model(&store.DungeonChest{}).
		Select("dungeon_chests.*, dungeon_runs.dungeon_type, dungeon_runs.dungeon_tier").
		Joins("JOIN dungeon_runs ON dungeon_runs.run_id = dungeon_chests.run_id").
		Where("dungeon_chests.paid = ? AND dungeon_chests.player_uuid = ?", true, uuid).
		Find(&chests)

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
			price, ok := c.MarketCache.Price(itemID)
			if !ok {
				continue
			}
			itemPrice := market.ChestPriceItems[chest.DungeonType][strconv.Itoa(chest.DungeonTier)][reward]
			value := (int(price) - itemPrice) * qty

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
			Name:  fmt.Sprintf("%s (%.2f%%)", e.item, float32(e.count)/float32(runs)),
			Value: fmt.Sprintf("%s (x%d)", utils.ShortNumber(e.value), e.count),
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
		Title:       "Chest Items Report",
		Description: fmt.Sprintf("Item value for %s: %s\nRuns: %v (avg. %v/run)", displayName, utils.ShortNumber(totalValue), runs, utils.ShortNumber(totalValue/int(runs))),
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
		c.logger.Errorf("failed to respond to interaction: %v", err)
	}
}
