package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ItsLukV/guild-tracker/internal/store"
	"github.com/ItsLukV/guild-tracker/internal/utils"
	"github.com/bwmarrin/discordgo"
	"gorm.io/gorm"
)

var commands = []*discordgo.ApplicationCommand{
	{
		Name:        "loss",
		Description: "Check a players loss",
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
}

// handlers maps each command name to the function that runs it.
var handlers = map[string]func(db *gorm.DB, s *discordgo.Session, i *discordgo.InteractionCreate){
	"loss":        loss,
	"leaderboard": leaderboard,
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

	var embed *discordgo.MessageEmbed
	if _, ok := m["leaderboard"]; !ok {
		return
	}
	switch m["leaderboard"].StringValue() {
	case "Coins Spent":
		{
			type Result struct {
				PlayerUUID string
				Total      int
			}
			var results []Result
			db.Model(&store.DungeonChest{}).
				Select("player_uuid, SUM(price) as total").
				Where("paid = ?", true).
				Group("player_uuid").
				Limit(10).
				Order("total DESC").
				Scan(&results)

			var fields []*discordgo.MessageEmbedField
			for rank, r := range results {
				displayName, err := utils.UUIDToName(r.PlayerUUID)
				if err != nil {
					fmt.Println("error:", err)
					displayName = r.PlayerUUID
				}
				fields = append(fields, &discordgo.MessageEmbedField{
					Name:  fmt.Sprintf("#%d - %s", rank+1, displayName),
					Value: utils.ShortNumber(r.Total),
				})
			}
			embed = &discordgo.MessageEmbed{
				Title:  "Leaderboard - Coins Spent",
				Color:  0xf1c40f,
				Fields: fields,
			}
		}
	}
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}

func loss(db *gorm.DB, s *discordgo.Session, i *discordgo.InteractionCreate) {
	type Result struct {
		DungeonType string
		DungeonTier int
		Total       int
	}

	uuid := "5ef04c7a95ae4c9396cefe925e4d5833"

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
		fmt.Println("error:", err)
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

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
	if err != nil {
		log.Printf("failed to respond to interaction: %v", err)
	}

}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	db, err := store.OpenDB()
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		log.Fatal("DISCORD_TOKEN environment variable is not set")
	}

	guildID := os.Getenv("GUILD_ID")

	session, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("failed to create Discord session: %v", err)
	}

	session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h, ok := handlers[i.ApplicationCommandData().Name]; ok {
			h(db, s, i)
		}
	})

	session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Logged in as %s#%s", r.User.Username, r.User.Discriminator)
	})

	if err := session.Open(); err != nil {
		log.Fatalf("failed to open connection: %v", err)
	}
	defer session.Close()

	log.Println("Registering commands...")
	registered := make([]*discordgo.ApplicationCommand, len(commands))
	for idx, cmd := range commands {
		c, err := session.ApplicationCommandCreate(session.State.User.ID, guildID, cmd)
		if err != nil {
			log.Fatalf("failed to create command %q: %v", cmd.Name, err)
		}
		registered[idx] = c
	}

	log.Println("Bot is running. Press Ctrl+C to exit.")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("Removing commands...")
	for _, cmd := range registered {
		if err := session.ApplicationCommandDelete(session.State.User.ID, guildID, cmd.ID); err != nil {
			log.Printf("failed to delete command %q: %v", cmd.Name, err)
		}
	}

	log.Println("Shutting down.")

}
