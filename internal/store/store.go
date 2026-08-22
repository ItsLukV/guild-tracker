package store

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type FetcherRunMode string

const (
	Hourly FetcherRunMode = "hourly"
	Daily  FetcherRunMode = "daily"
)

type FetcherRun struct {
	Mode    FetcherRunMode `gorm:"primaryKey"` // "hourly" | "daily"
	RanAt   time.Time
	Success bool
	Message string // error text on failure, or a short summary on success
}

type StringSlice []string

func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

func (s *StringSlice) Scan(src any) error {
	if src == nil {
		*s = nil
		return nil
	}
	switch v := src.(type) {
	case []byte:
		return json.Unmarshal(v, s)
	case string:
		return json.Unmarshal([]byte(v), s)
	default:
		return errors.New("unsupported type for StringSlice")
	}
}

type Player struct {
	MinecraftUUID    string `gorm:"primaryKey"`
	Username         string
	InGuild          bool `gorm:"default:true"`
	DiscordSnowflake string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        gorm.DeletedAt `gorm:"index"`
	Profiles         []Profile      `gorm:"foreignKey:PlayerUUID;references:MinecraftUUID"`
	Gexp             []Gexp         `gorm:"foreignKey:PlayerUUID;references:MinecraftUUID"`
}

type Profile struct {
	PlayerUUID string
	ProfileID  string `gorm:"primaryKey"`
	Type       string
	CuteName   string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
	Chests     []DungeonChest `gorm:"foreignKey:ProfileID;references:ProfileID"`
	Dungeons   []DungeonStats `gorm:"foreignKey:ProfileID;references:ProfileID"`
}

type DungeonStats struct {
	gorm.Model
	ProfileID                      string `gorm:"index:idx_dungeon_owner,priority:1"`
	PlayerUUID                     string `gorm:"index:idx_dungeon_owner,priority:2"`
	CatacombsExperience            float32
	Secrets                        int
	CatacombsTierCompletions       map[string]float32 `gorm:"serializer:json"`
	MasterCatacombsTierCompletions map[string]float32 `gorm:"serializer:json"`
	ClassExperience                map[string]float32 `gorm:"serializer:json"`
}

type DungeonChest struct {
	gorm.Model
	ProfileID     string `gorm:"index:idx_chest_owner,priority:1"`
	PlayerUUID    string `gorm:"index:idx_chest_owner,priority:2"`
	RunID         string `gorm:"index"`
	ChestID       string `gorm:"index"`
	DungeonType   string
	DungeonTier   int
	TreasureType  string
	Quality       int
	ShinyEligible bool
	Paid          bool
	Rerolls       int
	Rewards       StringSlice `gorm:"type:text"`
	Price         int
}

type Gexp struct {
	gorm.Model
	PlayerUUID string    `gorm:"uniqueIndex:idx_gexp_owner,priority:1"`
	Ts         time.Time `gorm:"uniqueIndex:idx_gexp_owner,priority:2"`
	Gexp       int
}

func OpenDB() (*gorm.DB, error) {
	driverName := os.Getenv("DB_DRIVER")
	if driverName == "" {
		driverName = "sqlite"
	}

	var dialector gorm.Dialector
	switch driverName {
	case "sqlite":
		path := os.Getenv("SQLITE_PATH")
		if path == "" {
			path = "chest_tracker.db"
		}
		dialector = sqlite.Open(path)
	case "postgres":
		dsn := os.Getenv("DB_DSN")
		if dsn == "" {
			return nil, errors.New("DB_DSN must be set when DB_DRIVER=postgres")
		}
		dialector = postgres.Open(dsn)
	default:
		return nil, fmt.Errorf("unknown DB_DRIVER: %q", driverName)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Error),
	})
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&Player{}, &Profile{}, &DungeonChest{}, &DungeonStats{}, &Gexp{}, &FetcherRun{}); err != nil {
		return nil, err
	}

	return db, nil
}
