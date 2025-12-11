package main

import (
	migrator_config "feed/internal/config/migrator-config"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	cfg := migrator_config.MustLoad()

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s&x-migrations-table=%s",
		cfg.Db.User,
		cfg.Db.Password,
		cfg.Db.Host,
		cfg.Db.Port,
		cfg.Db.Name,
		cfg.Db.SSLMode,
		cfg.MigrationsTable)

	m, err := migrate.New("file://"+cfg.MigrationsPath, dsn)
	if err != nil {
		panic(fmt.Sprintf("failed to create migrate instance: %v", err))
	}

	defer m.Close()

	switch {
	case cfg.ForceVersion >= 0:
		migrator_config.HandleForce(m, cfg.ForceVersion)
	case cfg.DropFlag:
		migrator_config.HandleDrop(m)
	case cfg.VersionFlag:
		migrator_config.HandleVersion(m)
	case cfg.DownFlag:
		migrator_config.HandleDown(m, cfg.Steps)
	default:
		migrator_config.HandleUp(m)
	}
}
