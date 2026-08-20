package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ItsLukV/guild-tracker/internal/logging"
	"github.com/ItsLukV/guild-tracker/internal/store"
	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

var logger *zap.SugaredLogger

func main() {
	logger = logging.New()
	defer logger.Sync()

	db, err := store.OpenDB()
	if err != nil {
		logger.Fatalf("open db: %v", err)
	}

	marketCache.StartAutoRefresh(24*time.Hour, logger.Errorf)

	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		logger.Fatal("DISCORD_TOKEN environment variable is not set")
	}

	guildID := os.Getenv("GUILD_ID")

	session, err := discordgo.New("Bot " + token)
	if err != nil {
		logger.Fatalf("failed to create Discord session: %v", err)
	}

	session.AddHandler(pg.handleButton)
	session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}
		if h, ok := handlers[i.ApplicationCommandData().Name]; ok {
			h(db, s, i)
		}
	})

	session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		logger.Infof("Logged in as %s#%s", r.User.Username, r.User.Discriminator)
	})

	if err := session.Open(); err != nil {
		logger.Fatalf("failed to open connection: %v", err)
	}
	defer session.Close()

	pg.StartJanitor(session)

	logger.Info("Registering commands...")
	if _, err := session.ApplicationCommandBulkOverwrite(session.State.User.ID, guildID, commands); err != nil {
		logger.Fatalf("failed to register commands: %v", err)
	}

	logger.Info("Bot is running. Press Ctrl+C to exit.")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	logger.Info("Cleaning up paginators...")
	pg.mu.Lock()
	pg.ttl = 0
	pg.mu.Unlock()
	pg.sweep(session)

	logger.Info("Shutting down.")

}
