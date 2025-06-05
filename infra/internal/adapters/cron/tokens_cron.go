package cron

import (
	"context"
	"time"

	appCtx "github.com/KhoshMaze/khoshmaze-backend/internal/adapters/context"
	"github.com/KhoshMaze/khoshmaze-backend/internal/adapters/storage/types"
	timeutils "github.com/KhoshMaze/khoshmaze-backend/internal/adapters/time"
	"github.com/go-co-op/gocron"
	"gorm.io/gorm"
)

func SetTokenDeleterJob(db *gorm.DB, interval int) {
	job := gocron.NewScheduler(timeutils.TehranLoc)
	job.Every(interval).Hours().Do(func() {
		logger := appCtx.GetLogger(context.Background())
		logger.Info("STARTING TOKEN DELETER CRON JOB")
		err := db.Where("expires_at < ?", time.Now()).Delete(&types.TokenWhitelist{}).Error
		if err != nil {
			logger.Error("TOKEN DELETER CRON JOB FAILED", err)
		}
		logger.Info("TOKEN DELETER CRON JOB FINISHED")
	})
	job.StartAsync()
}
