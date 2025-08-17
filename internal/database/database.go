package database

import (
	"fmt"
	"livescribble/internal/utils"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Manager struct {
	DB *gorm.DB
}

func NewDatabaseManager() *Manager {
	return &Manager{}
}

func (dbm *Manager) Connect() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL environment variable not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return err
	}

	err = db.AutoMigrate(utils.User{}, utils.Document{})
	if err != nil {
		fmt.Printf("%s", err.Error())
	}
	dbm.DB = db
	return nil
}
func (dbm *Manager) Close() error {
	db, err := dbm.DB.DB()
	if err != nil {
		return err
	}
	return db.Close()
}
