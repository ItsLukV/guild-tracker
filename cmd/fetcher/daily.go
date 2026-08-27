package main

import (
	"time"

	"github.com/ItsLukV/guild-tracker/internal/store"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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
