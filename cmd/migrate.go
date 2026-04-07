package cmd

import (
	"fmt"

	"vyaliksupport/pkg/db/postgres"

	_ "github.com/lib/pq"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var migrateCmd = &cobra.Command{
	Use:  "migrate",
	RunE: runMigrate,
}

func runMigrate(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	lgCfg := zap.NewProductionConfig()
	lgCfg.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05")
	lg, err := lgCfg.Build()
	if err != nil {
		return fmt.Errorf("failed to build logger: %w", err)
	}
	defer lg.Sync()

	db, err := connectDB(cfg.DB.DSN, cfg.DB.Dialect)
	if err != nil {
		lg.Error("can't connect to DB", zap.String("dsn", cfg.DB.DSN), zap.Error(err))
		return err
	}
	defer db.Close()

	repo := postgres.NewReq(db, cfg.Bot.SubHost)
	if err = repo.Migrate(); err != nil {
		lg.Error("can't migrate", zap.Error(err))
		return err
	}

	lg.Info("migrate finished")
	return nil
}

func init() {
	rootCmd.AddCommand(migrateCmd)
}
