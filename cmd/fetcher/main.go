package main

import (
	"context"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/ItsLukV/guild-tracker/internal/store"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var uuids = []string{
	"74e8ed7b7ffb4d61ab59cfd42c086a42", // LukV
	"5ef04c7a95ae4c9396cefe925e4d5833", // EmilMZ
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := NewClient(os.Getenv("HYPIXEL_API_KEY"))

	db, err := store.OpenDB()
	if err != nil {
		log.Printf("Failed to open db: %v\n", err)
	}

	if err := checkForMissingPlayers(db, uuids); err != nil {
		log.Panicf("Failed to check for missing players: %s", err)
	}

	for _, p := range uuids {
		var out ProfilesResponse
		q := url.Values{"uuid": {p}}
		if err := client.get(ctx, "/v2/skyblock/profiles", q, &out); err != nil {
			log.Printf("%s", err)
		} else {
			if err := checkForMissingProfiles(db, p, out.Profiles); err != nil {
				log.Printf("Failed to check for missing profiles for %s:\n %s", p, err)
			}

			insertChestData(db, p, out.Profiles)
		}
	}

}

func checkForMissingPlayers(db *gorm.DB, uuids []string) error {
	players := make([]store.Player, len(uuids))
	for i, id := range uuids {
		players[i] = store.Player{MinecraftUUID: id}
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(&players, 1000).
		Error
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

func insertChestData(db *gorm.DB, playerUUID string, profiles []Profile) error {
	var skipped, inserted int

	for _, profile := range profiles {
		if _, ok := profile.Members[playerUUID]; !ok {
			log.Println("Failed to get playerdata for %s in profile %s", playerUUID, profile.CuteName)
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
				ProfileID:     playerUUID,
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
				// Price:   0,
			})
			if result.Error != nil {
				log.Println("Failed to insert Chest data for %s\n%s", playerUUID, result.Error)
				return result.Error
			}
			if result.RowsAffected == 0 {
				skipped++
			} else {
				inserted++
			}
		}
	}

	log.Printf("Saved data for player %s: %d new chests (%d already recorded)\n", playerUUID, inserted, skipped)
	return nil
}
