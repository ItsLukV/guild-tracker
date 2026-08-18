package main

import (
	"context"
	"flag"
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ItsLukV/guild-tracker/internal/store"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	mode := flag.String("mode", "", "what to fetch: hourly | daily")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	client := NewClient(os.Getenv("HYPIXEL_API_KEY"))

	db, err := store.OpenDB()
	if err != nil {
		log.Printf("Failed to open db: %v\n", err)
	}

	members, err := fetchGuildUUIDs(ctx, client, "Specialstyrken")
	if err != nil {
		log.Printf("Failed to fetch guild members: %s", err)
	}

	uuids, err := checkForMissingPlayers(db, members)
	if err != nil {
		log.Fatalf("Failed to check for missing players: %s", err)
	}

	switch *mode {
	case "hourly":
		fetchHourly(ctx, db, client, uuids)
	case "daily":
		err = insertGEXP(db, members)
	default:
		log.Fatalf("unknown mode %q (want hourly or daily)", *mode)
	}

}

func checkForMissingPlayers(db *gorm.DB, guildInfo guildInfo) ([]store.Player, error) {
	players := make([]store.Player, len(guildInfo.Guild.Members))
	for i, id := range guildInfo.Guild.Members {
		players[i] = store.Player{MinecraftUUID: id.UUID}
	}
	err := db.Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(&players, 1000).
		Error
	return players, err
}

func fetchHourly(ctx context.Context, db *gorm.DB, client *Client, uuids []store.Player) {
	for _, player := range uuids {
		uuid := player.MinecraftUUID

		var out ProfilesResponse
		q := url.Values{"uuid": {uuid}}
		if err := client.get(ctx, "/v2/skyblock/profiles", q, &out); err != nil {
			log.Printf("%s", err)
		} else {
			if err := checkForMissingProfiles(db, uuid, out.Profiles); err != nil {
				log.Printf("Failed to check for missing profiles for %s:\n %s", uuid, err)
			}

			var outSkipped, outInserted int
			skipped, inserted, _ := insertChestData(db, uuid, out.Profiles)
			outSkipped += skipped
			outInserted += inserted
			skipped, inserted, _ = insertDungeonStats(db, uuid, out.Profiles)
			outSkipped += skipped
			outInserted += inserted
			log.Printf("Saved data for player %s: %d rows (%d already recorded)\n", uuid, outInserted, outSkipped)

		}
	}
}

func checkForMissingProfiles(db *gorm.DB, playerUUID string, profiles []Profile) error {
	store_profiles := make([]store.Profile, 0)

	for _, profile := range profiles {
		store_profiles = append(store_profiles, store.Profile{
			PlayerUUID: playerUUID,
			ProfileID:  profile.ProfileID,
			Type:       profile.GameMode(),
			CuteName:   profile.CuteName,
		})
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(&store_profiles, 1000).Error
}

func insertChestData(db *gorm.DB, playerUUID string, profiles []Profile) (skipped, inserted int, err error) {
	for _, profile := range profiles {
		if _, ok := profile.Members[playerUUID]; !ok {
			log.Printf("Failed to get playerdata for %s in profile %s", playerUUID, profile.CuteName)
		}

		treasures := profile.Members[playerUUID].Dungeons.Treasures
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
			result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&store.DungeonChest{
				PlayerUUID:    playerUUID,
				ProfileID:     profile.ProfileID,
				RunID:         chest.RunId,
				ChestID:       chest.ChestId,
				DungeonType:   runTier[chest.RunId].runType,
				DungeonTier:   runTier[chest.RunId].runTier,
				TreasureType:  chest.TreasureType,
				Quality:       chest.Quality,
				ShinyEligible: chest.ShinyEligible,
				Paid:          chest.Paid,
				Rerolls:       chest.Rerolls,
				Rewards:       chest.Rewards.Rewards,
				Price:         ChestPrice(chest.TreasureType, runTier[chest.RunId].runTier, chest.Rewards.Rewards),
			})
			if result.Error != nil {
				log.Printf("Failed to insert Chest data for %s\n%s", playerUUID, result.Error)
				return skipped, inserted, result.Error
			}
			if result.RowsAffected == 0 {
				skipped++
			} else {
				inserted++
			}
		}
	}

	return skipped, inserted, err
}

func insertDungeonStats(db *gorm.DB, playerUUID string, profiles []Profile) (skipped, inserted int, err error) {
	for _, profile := range profiles {
		player := profile.Members[playerUUID]

		classExp := make(map[string]float32)
		for k, e := range player.Dungeons.PlayerClasses {
			classExp[k] = e.Experience
		}

		result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&store.DungeonStats{
			ProfileID:                      profile.ProfileID,
			PlayerUUID:                     playerUUID,
			CatacombsExperience:            player.Dungeons.DungeonTypes.Catacombs.Experience,
			Secrets:                        player.Dungeons.Secrets,
			CatacombsTierCompletions:       player.Dungeons.DungeonTypes.Catacombs.TierCompletions,
			MasterCatacombsTierCompletions: player.Dungeons.DungeonTypes.MasterCatacombs.TierCompletions,
			ClassExperience:                classExp,
		})
		if result.Error != nil {
			log.Printf("Failed to insert Chest data for %s\n%s", playerUUID, result.Error)
			return skipped, inserted, result.Error
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
	log.Println("Starting to insert guild info")
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
				log.Println("fAILED with inserting guild info\n%s", err)
				return err
			}
		}
	}
	log.Println("Done with inserting guild info")
	return nil
}
