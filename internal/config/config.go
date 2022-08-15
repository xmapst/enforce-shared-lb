package config

import (
	"encoding/json"
	"os"

	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
)

// Configure stores configuration.
type Configure struct {
	Debug       bool              `json:"debug" default:"false"`
	Addr        string            `json:"addr" default:"0.0.0.0"`
	Port        int64             `json:"port" default:"8080"`
	ChannelSize int               `json:"channel_size" default:"1024"`
	Redis       string            `json:"redis" default:"redis://:123456@localhost:6379/0"`
	KeyPrefix   string            `json:"key_prefix" default:"enforce_shared_lb"`
	Labels      map[string]string `json:"labels" default:"lb_address_type:internet,q1autoops_type:game-service"`
	Cloud       *Cloud            `json:"cloud"`
	// 预留自用
	CloudConf interface{} `json:"-"`
}

type Cloud struct {
	Name            string          `json:"name" default:"alibaba"`
	Max             int64           `json:"max" default:"51"`
	Endpoint        *string         `json:"endpoint" default:""`
	AccessKeyId     *string         `json:"access_key_id" default:""`
	AccessKeySecret *string         `json:"access_key_secret" default:""`
	Config          json.RawMessage `json:"config"`
}

var (
	Conf = &Configure{
		Debug:       false,                              //default false
		Addr:        "0.0.0.0",                          //default 0.0.0.0
		Port:        8080,                               // default 8080
		ChannelSize: 409600,                             //default 409600
		Redis:       "redis://:123456@localhost:6379/0", // default "redis://:123456@localhost:6379/0"
		KeyPrefix:   "enforce_shared_lb",                // default enforce_shared_lb
		Cloud:       new(Cloud),
	}
	path = kingpin.Flag("config", "Configure file path").Short('c').Default("config.json").String()
)

// Init config.
func Init() {
	file, err := os.ReadFile(*path)
	if err != nil {
		logrus.Fatalln(err)
	}
	err = json.Unmarshal(file, Conf)
	if err != nil {
		logrus.Fatalln(err)
	}
	Conf.loadCloudConf()

	// init redis
	err = Conf.newRedisClient()
	if err != nil {
		logrus.Fatalln(err)
	}
	err = Conf.newKubeClient()
	if err != nil {
		logrus.Warning(err)
	}
}
