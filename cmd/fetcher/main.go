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

	"github.com/ItsLukV/guild-tracker/internal/market"

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

func (p *Profile) insertChestData(db *gorm.DB, playerUUID string) (skipped, inserted int, err error) {
	if _, ok := p.Members[playerUUID]; !ok {
		logger.Errorf("Failed to get playerdata for %s in profile %s", playerUUID, p.CuteName)
		return 0, 0, err
	}

	treasures := p.Members[playerUUID].Dungeons.Treasures
	type runInfo struct {
		runTier int
		runType string
	}

	runTier := make(map[string]runInfo)
	for _, run := range treasures.Runs {
		runTier[run.RunId] = runInfo{
			runTier: run.DungeonTier,
			runType: run.DungeonType,
		}
	}
	for _, chest := range treasures.Chests {
		if chest.Type != "DUNGEON" {
			continue
		}

		result := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "player_uuid"}, {Name: "chest_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"paid", "rerolls", "rewards", "price"}),
			Where: clause.Where{Exprs: []clause.Expression{clause.Expr{
				SQL: "dungeon_chests.paid <> excluded.paid OR dungeon_chests.rerolls <> excluded.rerolls OR dungeon_chests.rewards <> excluded.rewards OR dungeon_chests.price <> excluded.price",
			}}},
		}).Create(&store.DungeonChest{
			PlayerUUID:    playerUUID,
			ProfileID:     p.ProfileID,
			RunID:         chest.RunId,
			ChestID:       chest.ChestId,
			TreasureType:  chest.TreasureType,
			Quality:       chest.Quality,
			ShinyEligible: chest.ShinyEligible,
			Paid:          chest.Paid,
			Rerolls:       chest.Rerolls,
			Rewards:       chest.Rewards.Rewards,
			Price:         market.ChestPrice(chest.TreasureType, runTier[chest.RunId].runTier, chest.Rewards.Rewards),
		})
		if result.Error != nil {
			logger.Errorf("Failed to insert Chest data for %s: %v", playerUUID, result.Error)
			return skipped, inserted, result.Error
		}
		if result.RowsAffected == 0 {
			skipped++
		} else {
			inserted++
		}
	}

	return skipped, inserted, err
}

func (p *Profile) insertDungeonStats(db *gorm.DB, playerUUID string) (skipped, inserted int, err error) {
	player := p.Members[playerUUID]

	classExp := make(map[string]float32)
	for k, e := range player.Dungeons.PlayerClasses {
		classExp[k] = e.Experience
	}

	result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&store.DungeonStats{
		ProfileID:                      p.ProfileID,
		PlayerUUID:                     playerUUID,
		CatacombsExperience:            player.Dungeons.DungeonTypes.Catacombs.Experience,
		Secrets:                        player.Dungeons.Secrets,
		CatacombsTierCompletions:       player.Dungeons.DungeonTypes.Catacombs.TierCompletions,
		MasterCatacombsTierCompletions: player.Dungeons.DungeonTypes.MasterCatacombs.TierCompletions,
		ClassExperience:                classExp,
	})
	if result.Error != nil {
		logger.Errorf("Failed to insert dungeon stats for %s: %v", playerUUID, result.Error)
		return skipped, inserted, result.Error
	}
	if result.RowsAffected == 0 {
		skipped++
	} else {
		inserted++
	}
	return skipped, inserted, nil
}

func (p *Profile) insertDungeonsRuns(db *gorm.DB, playerUUID string) (skipped, inserted int, err error) {
	for _, run := range p.Members[playerUUID].Dungeons.Treasures.Runs {
		if run.Type != "DUNGEON" {
			// logger.Warnf("Skipping non dungeon treasure run for player %s", playerUUID)
			continue
		}

		result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(
			&store.DungeonRun{
				RunId:        run.RunId,
				CompletionTs: time.Unix(run.CompletionTs, 0),
				DungeonType:  run.DungeonType,
				DungeonTier:  run.DungeonTier,
				ProfileID:    p.ProfileID,
			})
		if result.Error != nil {
			logger.Errorf("Failed to insert dungeon stats for %s: %v", playerUUID, result.Error)
			return skipped, inserted, result.Error
		}

		participants := make([]store.Participant, 0, len(run.Participants))
		for _, participant := range run.Participants {
			participants = append(participants, participant.toStore(run.RunId))
		}
		if len(participants) > 0 {
			if err := db.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "dungeon_run_id"}, {Name: "player_uuid"}},
				DoNothing: true,
			}).Create(&participants).Error; err != nil {
				logger.Errorf("Failed to insert participants for run %s: %v", run.RunId, err)
				return skipped, inserted, err
			}
		}

		if result.RowsAffected == 0 {
			skipped++
		} else {
			inserted++
		}
	}
	return skipped, inserted, nil
}

func insertGEXP(db *gorm.DB, guildInfo guildInfo) error {
	logger.Info("Starting to insert guild info")
	for _, member := range guildInfo.Guild.Members {
		for k, v := range member.ExpHistory {
			if err := db.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "player_uuid"}, {Name: "ts"}},
				DoUpdates: clause.AssignmentColumns([]string{"gexp"}),
			}).Create(&store.Gexp{
				PlayerUUID: member.UUID,
				Ts:         time.Time(k),
				Gexp:       v,
			}).Error; err != nil {
				logger.Errorf("Failed to insert guild info: %v", err)
				return err
			}
		}
	}
	logger.Info("Done with inserting guild info")
	return nil
}
