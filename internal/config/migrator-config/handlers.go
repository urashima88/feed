package migrator_config

import (
	"errors"
	"fmt"
	"log"

	"github.com/golang-migrate/migrate/v4"
)

func HandleUp(m *migrate.Migrate) {
	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			log.Println("no migrations to apply")
			return
		}
		panic(fmt.Sprintf("failed to apply migrations: %v", err))
	}
	log.Println("migrations applied successfully")
}

func HandleForce(m *migrate.Migrate, version int) {
	if err := m.Force(version); err != nil {
		panic(fmt.Sprintf("failed to force version: %v", err))
	}
	log.Printf("successfully forced version to %d", version)
}

func HandleDrop(m *migrate.Migrate) {
	if err := m.Drop(); err != nil {
		panic(fmt.Sprintf("failed to drop: %v", err))
	}
	log.Println("successfully dropped all migrations")
}

func HandleVersion(m *migrate.Migrate) {
	version, dirty, err := m.Version()
	if err != nil {
		if errors.Is(err, migrate.ErrNilVersion) {
			log.Println("no migrations applied")
			return
		}
		panic(fmt.Sprintf("failed to get version: %v", err))
	}

	status := "clean"
	if dirty {
		status = "dirty"
	}
	log.Printf("current version: %d (%s)", version, status)
}

func HandleDown(m *migrate.Migrate, steps int) {
	if err := m.Steps(-steps); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			log.Println("no migrations to rollback")
			return
		}
		panic(fmt.Sprintf("failed to rollback migrations: %v", err))
	}
	log.Printf("successfully rolled back %d migration(s)", steps)
}
