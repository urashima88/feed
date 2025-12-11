package migrator_config

import (
	"flag"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Db
	MigrationsPath  string
	MigrationsTable string
	ForceVersion    int
	Steps           int
	DropFlag        bool
	VersionFlag     bool
	DownFlag        bool
}

type Db struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

func MustLoad() *Config {
	var (
		configPath, migrationsPath, migrationsTable string
		forceVersion, steps                         int
		dropFlag, versionFlag, downFlag             bool
	)

	flag.StringVar(&configPath, "config", "", "path to config file")
	flag.StringVar(&migrationsPath, "migrations-path", "", "path to migrations")
	flag.StringVar(&migrationsTable, "migrations-table", "migrations", "name of migrations table")

	flag.IntVar(&forceVersion, "force", -1, "force set database version")
	flag.IntVar(&steps, "steps", 1, "number of migrations to rollback (with -down)")
	flag.BoolVar(&dropFlag, "drop", false, "drop everything and start fresh")
	flag.BoolVar(&versionFlag, "version", false, "show current migration version")
	flag.BoolVar(&downFlag, "down", false, "rollback migrations")

	flag.Parse()

	if configPath == "" {
		panic("config file is empty")
	}

	viper.SetConfigFile(configPath)
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := viper.ReadInConfig(); err != nil {
		panic("error reading config file: " + err.Error())
	}

	if migrationsPath == "" {
		panic("migrations-path is required")
	}

	cfg := Config{
		Db: Db{
			Host:     viper.GetString("DB_HOST"),
			Port:     viper.GetString("DB_PORT"),
			User:     viper.GetString("DB_USER"),
			Password: viper.GetString("DB_PASSWORD"),
			Name:     viper.GetString("DB_NAME"),
			SSLMode:  viper.GetString("SSL_MODE"),
		},
		MigrationsPath:  migrationsPath,
		MigrationsTable: migrationsTable,
		ForceVersion:    forceVersion,
		Steps:           steps,
		DropFlag:        dropFlag,
		VersionFlag:     versionFlag,
		DownFlag:        downFlag,
	}

	return &cfg
}
