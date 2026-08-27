package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ItsLukV/guild-tracker/internal/logging"
	"github.com/ItsLukV/guild-tracker/internal/store"
	"github.com/ItsLukV/guild-tracker/internal/utils"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var logger *zap.SugaredLogger

func main() {
	logger = logging.New()
	defer logger.Sync()

	mode := flag.String("mode", "", "what to fetch: hourly | daily")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	client := NewClient(os.Getenv("HYPIXEL_API_KEY"))

	db, err := store.OpenDB()
	if err != nil {
		logger.Errorf("Failed to open db: %v", err)
	}

	members, err := fetchGuildUUIDs(ctx, client, "Specialstyrken")
	if err != nil {
		logger.Errorf("Failed to fetch guild members: %v", err)
	}

	uuids, err := syncGuildMembers(db, members)
	if err != nil {
		logger.Fatalf("Failed to sync guild members: %v", err)
	}

	var runErr error
	switch store.FetcherRunMode(*mode) {
	case store.Hourly:
		logger.Info("Started hourly fetching")
		runErr = fetchHourly(ctx, db, client, uuids)
	case store.Daily:
		logger.Info("Started daily fetching")
		runErr = insertGEXP(db, members)
	default:
		logger.Fatalf("unknown mode %q (want hourly or daily)", *mode)
	}

	db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "mode"}},
		DoUpdates: clause.AssignmentColumns([]string{"ran_at", "success", "message"}),
	}).Create(&store.FetcherRun{
		Mode:    store.FetcherRunMode(*mode),
		RanAt:   time.Now(),
		Success: runErr == nil,
		Message: fmt.Sprintf("%v", runErr),
	})
}

func syncGuildMembers(db *gorm.DB, guildInfo guildInfo) ([]store.Player, error) {
	uuids := make([]string, len(guildInfo.Guild.Members))
	for i, m := range guildInfo.Guild.Members {
		uuids[i] = m.UUID
	}

	if err := db.Model(&store.Player{}).
		Where("in_guild = ? AND minecraft_uuid NOT IN ?", true, uuids).
		Update("in_guild", false).Error; err != nil {
		return nil, fmt.Errorf("mark departed players: %w", err)
	}

	var existing []store.Player
	if err := db.Where("minecraft_uuid IN ?", uuids).Find(&existing).Error; err != nil {
		return nil, fmt.Errorf("load existing players: %w", err)
	}
	existingByUUID := make(map[string]store.Player, len(existing))
	for _, p := range existing {
		existingByUUID[p.MinecraftUUID] = p
	}

	const lookupSpacing = 1200 * time.Millisecond

	players := make([]store.Player, len(guildInfo.Guild.Members))
	for i, m := range guildInfo.Guild.Members {
		p, ok := existingByUUID[m.UUID]
		if !ok || p.Username == "" {
			if i > 0 {
				time.Sleep(lookupSpacing)
			}
			name, err := utils.UUIDToName(m.UUID)
			if err != nil {
				logger.Errorf("failed to resolve username for %s: %v", m.UUID, err)
				name = p.Username
			} else {
				p.Username = name
			}
		}
		p.MinecraftUUID = m.UUID
		p.InGuild = true
		players[i] = p
	}

	err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "minecraft_uuid"}},
		DoUpdates: clause.AssignmentColumns([]string{"username", "in_guild"}),
	}).CreateInBatches(&players, 1000).Error
	return players, err
}

func fetchHourly(ctx context.Context, db *gorm.DB, client *Client, uuids []store.Player) (err error) {
	totalPlayers := len(uuids)
	for i, player := range uuids {
		uuid := player.MinecraftUUID

		var out ProfilesResponse
		q := url.Values{"uuid": {uuid}}
		if err := client.get(ctx, "/v2/skyblock/profiles", q, &out); err != nil {
			logger.Errorf("%v", err)
		} else {
			if err := checkForMissingProfiles(db, uuid, out.Profiles); err != nil {
				err = fmt.Errorf("failed to check for missing profiles for %s: %v", uuid, err)
				logger.Errorf("%v", err)
			}

			var outSkipped, outInserted int
			for _, profile := range out.Profiles {
				// TODO make this prettier
				skipped, inserted, _ := profile.insertChestData(db, uuid)
				outSkipped += skipped
				outInserted += inserted
				skipped, inserted, _ = profile.insertDungeonsRuns(db, uuid)
				outSkipped += skipped
				outInserted += inserted
				skipped, inserted, _ = profile.insertDungeonStats(db, uuid)
				outSkipped += skipped
				outInserted += inserted
			}
			logger.Infof("[%v/%v] Saved data for player %s: %d rows (%d already recorded)", i, totalPlayers-1, player.Username, outInserted, outSkipped)
		}
	}
	return err
}

func checkForMissingProfiles(db *gorm.DB, playerUUID string, profiles []Profile) error {
	storeProfiles := make([]store.Profile, 0)

	for _, profile := range profiles {
		storeProfiles = append(storeProfiles, store.Profile{
			PlayerUUID: playerUUID,
			ProfileID:  profile.ProfileID,
			Type:       profile.GameMode(),
			CuteName:   profile.CuteName,
		})
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(&storeProfiles, 1000).Error
}
