package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kelseyhightower/envconfig"
	"github.com/pborman/getopt/v2"
	"gopkg.in/yaml.v2"
)

const configFileName = "config.yaml"

// ServerConfig : structure to hold configuration
type MQTTConfig struct {
	HOST     string `yaml:"host" envconfig:"MQTT_BROKER_HOST"`
	PORT     uint   `yaml:"port" envconfig:"MQTT_BROKER_PORT"`
	USER     string `yaml:"user" envconfig:"MQTT_BROKER_USER"`
	PASSWORD string `yaml:"password" envconfig:"MQTT_BROKER_PASSWORD"`
}

type SentryConfig struct {
	DSN         string `yaml:"dsn" envconfig:"SENTRY_BACKEND_DSN"`
	Environment string `yaml:"environment" envconfig:"SENTRY_ENVIRONMENT"`
	Enable      bool   `yaml:"enable" envconfig:"SENTRY_ENABLE"`
}

type MySQLConfig struct {
	DATABASE string `yaml:"database" envconfig:"DB_DATABASE"`
	HOST     string `yaml:"host" envconfig:"DB_HOST"`
	PORT     uint   `yaml:"port" envconfig:"DB_PORT"`
	USERNAME string `yaml:"username" envconfig:"DB_USERNAME"`
	PASSWORD string `yaml:"password" envconfig:"DB_PASSWORD"`
}

type KeycloakConfig struct {
	SERVER string `yaml:"server" envconfig:"KEYCLOAK_SERVER"`
	SECRET string `yaml:"secret" envconfig:"KEYCLOAK_SECRET"`
	CLIENT string `yaml:"client" envconfig:"KEYCLOAK_CLIENT"`
	REALM  string `yaml:"realm" envconfig:"KEYCLOAK_REALM"`
}

type LocalConfig struct {
	Address   string `yaml:"bind_address" envconfig:"RENDER_BIND_ADDRESS"`
	Port      uint   `yaml:"bind_port" envconfig:"RENDER_BIND_PORT"`
	Fontpath  string `yaml:"fontpath" envconfig:"RENDER_FONT_PATH"`
	Imagepath string `yaml:"image_path" envconfig:"RENDER_IMAGE_PATH"`
	Audiopath string `yaml:"audio_path" envconfig:"RENDER_AUDIO_PATH"`
	LogLevel  int8   `yaml:"loglevel"  envconfig:"RENDER_LOGLEVEL"`
}

func (x *MQTTConfig) Init() {
	x.HOST = "localhost"
	x.PORT = 1883
	x.USER = ""
	x.PASSWORD = ""
}

func (x *SentryConfig) Init() {
	dbase := os.Getenv("DEPLOYMENT_BASE_URL")
	if dbase != "" {
		i1 := strings.Index(dbase, "://")
		if i1 < 0 {
			i1 = 0
		} else {
			i1 += 3
		}
		dbase1 := dbase[i1:]
		i2 := strings.Index(dbase1, ".")

		if i2 < 0 {
			i2 = len(dbase1)
		}
		x.Environment = dbase1[:i2]
	}
	x.Enable = true
}

func (x *MySQLConfig) Init() {
	x.DATABASE = "momentum2"
	x.HOST = "localhost"
	x.PASSWORD = ""
	x.USERNAME = "root"
	x.PORT = 3306
}

func (x *KeycloakConfig) Init() {
	x.SERVER = ""
	x.SECRET = ""
	x.CLIENT = "nest-microservices"
	x.REALM = "Momentum"
}

func (x *LocalConfig) Init() {
	x.Address = "0.0.0.0"
	x.Port = 4000
	x.Fontpath = "./fonts"
	x.Imagepath = "./images"
	x.Audiopath = "./images/tracks"
	x.LogLevel = 0
}

// Config : structure to hold configuration
type Config struct {
	MQTT     MQTTConfig     `yaml:"mqtt"`
	MySQL    MySQLConfig    `yaml:"mysql"`
	KeyCloak KeycloakConfig `yaml:"keycloak"`
	Sentry   SentryConfig   `yaml:"sentry"`
	Settings LocalConfig    `yaml:"settings"`
}

func (x *Config) Init() {
	x.MQTT.Init()
	x.MySQL.Init()
	x.KeyCloak.Init()
	x.Sentry.Init()
	x.Settings.Init()
}

func defConfig() Config {
	var cfg Config
	cfg.Init()
	return cfg
}

func readOpts(cfg *Config) {
	helpFlag := false
	getopt.Flag(&helpFlag, 'h', "display help")
	getopt.Flag(&cfg.Settings.LogLevel, 'l', "be verbose")
	getopt.FlagLong(&cfg.Settings.Fontpath, "fontpath", 'f', "Path to fonts")
	getopt.FlagLong(&cfg.Settings.Imagepath, "imagepath", 'i', "Path to rendered images")
	getopt.FlagLong(&cfg.Settings.Audiopath, "audiopath", 'a', "Path to rendered images")
	getopt.FlagLong(&cfg.Settings.Port, "port", 'p', "Listen port")

	getopt.Parse()
	if helpFlag {
		getopt.Usage()
		os.Exit(0)
	}
}

func processError(err error) {
	fmt.Println(err)
	os.Exit(2)
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func readFile(cfg *Config) {
	if !fileExists(configFileName) {
		return
	}
	f, err := os.Open(configFileName)
	if err != nil {
		processError(err)
	}
	defer f.Close()
	decoder := yaml.NewDecoder(f)
	err = decoder.Decode(cfg)
	if err != nil {
		if err != io.EOF {
			processError(err)
		}
	}
}

func readEnv(cfg *Config) {
	err := envconfig.Process("", cfg)
	if err != nil {
		processError(err)
	}
}

func prettyPrint(cfg *Config) {
	d, _ := yaml.Marshal(cfg)
	L().Info("--- Config ---\n%s\n\n", string(d))
}

// GetConfig : get config file
func GetConfig() Config {
	cfg := defConfig()

	readFile(&cfg)
	readEnv(&cfg)
	readOpts(&cfg)

	prettyPrint(&cfg)
	return cfg
}
