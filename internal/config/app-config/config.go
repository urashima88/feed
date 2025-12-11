package app_config

import (
	"flag"
	"slices"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Env string
	Db
	HTTPServer
	PostMeta
	ImageService
	TagService
}

type HTTPServer struct {
	Host        string
	Port        string
	Timeout     time.Duration
	IdleTimeout time.Duration
}

type Db struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

type PostMeta struct {
	MaxNumberImages int
	MaxTextLength   int
	MaxTagsPerImage int
	MaxTagLength    int
}

type ImageService struct {
	Host         string
	Port         string
	Timeout      time.Duration
	GetImagesURL string
}

type TagService struct {
	Host          string
	Port          string
	Timeout       time.Duration
	CreateTagsURL string
	GetTagsURL    string
}

var envs = []string{"local", "dev", "prod"}

func MustLoad() *Config {
	var configPath, env string
	flag.StringVar(&configPath, "config", "", "path to config file")
	flag.StringVar(&env, "env", "", "environment (local, dev, prod)")
	flag.Parse()

	if configPath == "" {
		panic("config file is empty")
	}

	if !slices.Contains(envs, env) {
		panic("env doesn't match one of the values: local, dev, prod")
	}

	viper.SetConfigFile(configPath)
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := viper.ReadInConfig(); err != nil {
		panic("error reading config file: " + err.Error())
	}

	cfg := Config{
		Env: env,
		HTTPServer: HTTPServer{
			Host:        viper.GetString("APP_HOST"),
			Port:        viper.GetString("APP_PORT"),
			Timeout:     viper.GetDuration("APP_TIMEOUT"),
			IdleTimeout: viper.GetDuration("APP_IDLE_TIMEOUT"),
		},
		Db: Db{
			Host:     viper.GetString("DB_HOST"),
			Port:     viper.GetString("DB_PORT"),
			User:     viper.GetString("DB_USER"),
			Password: viper.GetString("DB_PASSWORD"),
			Name:     viper.GetString("DB_NAME"),
			SSLMode:  viper.GetString("SSL_MODE"),
		},
		PostMeta: PostMeta{
			MaxNumberImages: viper.GetInt("POST_MAX_NUMBER_IMAGES"),
			MaxTextLength:   viper.GetInt("POST_MAX_TEXT_LENGTH"),
			MaxTagsPerImage: viper.GetInt("POST_MAX_TAGS_PER_IMAGE"),
			MaxTagLength:    viper.GetInt("POST_MAX_TAG_LENGTH"),
		},
		ImageService: ImageService{
			Host:         viper.GetString("IMAGE_SERVICE_HOST"),
			Port:         viper.GetString("IMAGE_SERVICE_PORT"),
			Timeout:      viper.GetDuration("IMAGE_SERVICE_TIMEOUT"),
			GetImagesURL: viper.GetString("GET_IMAGES_URL"),
		},
		TagService: TagService{
			Host:          viper.GetString("TAG_SERVICE_HOST"),
			Port:          viper.GetString("TAG_SERVICE_PORT"),
			Timeout:       viper.GetDuration("TAG_SERVICE_TIMEOUT"),
			CreateTagsURL: viper.GetString("CREATE_TAGS_URL"),
			GetTagsURL:    viper.GetString("GET_TAGS_URL"),
		},
	}

	return &cfg
}
