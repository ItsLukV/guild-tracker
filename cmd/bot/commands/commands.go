package commands

import (
	"time"

	"github.com/ItsLukV/guild-tracker/cmd/bot/paginator"
	"github.com/ItsLukV/guild-tracker/internal/market"
	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Commands struct {
	MarketCache *market.Cache
	logger      *zap.SugaredLogger
	pg          *paginator.Paginator
	db          *gorm.DB
	handlers    map[string]func(db *gorm.DB, s *discordgo.Session, i *discordgo.InteractionCreate)
}

func NewCommands(logger *zap.SugaredLogger, paginator *paginator.Paginator, db *gorm.DB) *Commands {

	c := &Commands{
		MarketCache: market.NewCache(),
		logger:      logger,
		pg:          paginator,
		db:          db,
	}

	c.handlers = map[string]func(db *gorm.DB, s *discordgo.Session, i *discordgo.InteractionCreate){
		"loss":        c.loss,
		"items":       c.items,
		"leaderboard": c.leaderboard,
		"inactivity":  c.inactivity,
		"status":      c.status,
	}
	return c
}

var List = []*discordgo.ApplicationCommand{
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

func (c *Commands) HandleCommands(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	if h, ok := c.handlers[i.ApplicationCommandData().Name]; ok {
		h(c.db, s, i)
	}
}

func (c *Commands) StartMarketCacheRefresh(interval time.Duration, logf func(format string, v ...any)) {
	c.MarketCache.StartAutoRefresh(interval, logf)
}

func (c *Commands) sendFailedEmbed(msg string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "Error",
		Color:       0xfc0000,
		Description: msg,
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}
