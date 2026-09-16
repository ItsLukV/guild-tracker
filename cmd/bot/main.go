package main

import (
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	com "github.com/ItsLukV/guild-tracker/cmd/bot/commands"
	"github.com/ItsLukV/guild-tracker/cmd/bot/paginator"
	"github.com/ItsLukV/guild-tracker/internal/logging"
	"github.com/ItsLukV/guild-tracker/internal/store"
	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

const gatewayHealthyWindow = 15 * time.Minute

var logger *zap.SugaredLogger

var commands *com.Commands
var pg *paginator.Paginator

var lastGatewayConnectUnixNano atomic.Int64

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	last := time.Unix(0, lastGatewayConnectUnixNano.Load())
	if time.Since(last) > gatewayHealthyWindow {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func startHealthServer(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthzHandler)

	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			logger.Errorf("health server stopped: %v", err)
		}
	}()
}

func main() {
	logger = logging.New()
	defer logger.Sync()

	lastGatewayConnectUnixNano.Store(time.Now().UnixNano())
	startHealthServer(":8081")

	db, err := store.OpenDB()
	if err != nil {
		logger.Fatalf("open db: %v", err)
	}
	pg = paginator.NewPaginator(logger)

	commands = com.NewCommands(logger, pg, db)
	commands.StartMarketCacheRefresh(time.Minute*15, logger.Errorf)

	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		logger.Fatal("DISCORD_TOKEN environment variable is not set")
	}

	guildID := os.Getenv("GUILD_ID")

	session, err := discordgo.New("Bot " + token)
	if err != nil {
		logger.Fatalf("failed to create Discord session: %v", err)
	}

	session.AddHandler(pg.HandleButton)
	session.AddHandler(commands.HandleCommands)

	session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		logger.Infof("Logged in as %s#%s", r.User.Username, r.User.Discriminator)
		lastGatewayConnectUnixNano.Store(time.Now().UnixNano())
	})
	session.AddHandler(func(s *discordgo.Session, r *discordgo.Resumed) {
		lastGatewayConnectUnixNano.Store(time.Now().UnixNano())
	})

	if err := session.Open(); err != nil {
		logger.Fatalf("failed to open connection: %v", err)
	}
	defer session.Close()

	pg.StartJanitor(session)

	logger.Info("Registering commands...")
	if _, err := session.ApplicationCommandBulkOverwrite(session.State.User.ID, guildID, com.List); err != nil {
		logger.Fatalf("failed to register commands: %v", err)
	}

	logger.Info("Bot is running. Press Ctrl+C to exit.")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	logger.Info("Cleaning up paginators...")
	pg.FullSweep(session)
	logger.Info("Shutting down.")

}
