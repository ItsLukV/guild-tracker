package main

import (
	"time"

	"github.com/ItsLukV/guild-tracker/internal/market"
	"github.com/ItsLukV/guild-tracker/internal/store"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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
