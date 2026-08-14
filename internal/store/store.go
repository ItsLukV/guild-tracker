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
	DiscordSnowflake string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        gorm.DeletedAt `gorm:"index"`
	Profiles         []Profile      `gorm:"foreignKey:PlayerUUID;references:MinecraftUUID"`
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
}

type DungeonChest struct {
	gorm.Model
	ProfileID     string `gorm:"index"`
	RunID         string `gorm:"index"`
	ChestID       string `gorm:"uniqueIndex"`
	DungeonType   string
	DungeonTier   int
	TreasureType  string
	Quality       int
	ShinyEligible bool
	Paid          bool
	Rerolls       int
	Rewards       StringSlice `gorm:"type:text"`
	// Price         int // TODO: added price paided for chest
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

	if err := db.AutoMigrate(&Player{}, &Profile{}, &DungeonChest{}); err != nil {
		return nil, err
	}

	return db, nil
}

// func main() {
// 	db, err := gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
// 	if err != nil {
// 		panic("failed to connect database")
// 	}

// 	// Migrate the schema
// 	db.AutoMigrate(&Player{}, &Chests{}, &Test{})

// 	if err := db.Create(&Player{
// 		MinecraftUUID: "test",
// 		Chests:        []Chests{{ChestName: "chest"}, {ChestName: "chest2"}},
// 		Test:          []Test{{Test: "e"}},
// 	}).Error; err != nil {
// 		fmt.Println("insert failed:", err) // UNIQUE constraint failed: players.uuid
// 	}
// 	db.Create(&[]Test{
// 		{PlayerUUID: "test", Test: "e"},
// 		{PlayerUUID: "testasd", Test: "f"},
// 	})

// 	var player Player
// 	db.Preload("Test").Take(&player, "minecraft_uuid = ?", "test")
// 	b, _ := json.MarshalIndent(player, "", "  ")
// 	fmt.Println(string(b))
// }
