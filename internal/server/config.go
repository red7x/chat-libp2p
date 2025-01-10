package server

import (
	"chat-libp2p/pkg/logger"
	"fmt"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

type Body struct {
	Listen     []string `mapstructure:"listen"`
	Bootstrap  []string `mapstructure:"bootstrap"`
	PublicIP   string   `mapstructure:"publicIP"`
	PrivateKey string   `mapstructure:"privateKey"`
}

type Config struct {
	body   *Body
	mu     *sync.Mutex
	update []func(*Config) error
}

func NewConfig() *Config {
	config := &Config{
		body:   &Body{},
		mu:     &sync.Mutex{},
		update: make([]func(*Config) error, 0),
	}
	return config
}

func (c *Config) Load() (err error) {
	viper.SetConfigName("server")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	if err = viper.ReadInConfig(); err != nil {
		err = fmt.Errorf("failed to load server.yaml: %v", err)
		return
	} else if err = viper.Unmarshal(c.body); err != nil {
		err = fmt.Errorf("failed to unmarshal config: %v", err)
		return
	}

	ticker := time.NewTicker(time.Second * 2)
	viper.OnConfigChange(func(in fsnotify.Event) {
		select {
		case <-ticker.C:
			c.mu.Lock()
			if err := viper.Unmarshal(c.body); err != nil {
				c.mu.Unlock()
				logger.Error("failed to unmarshal config: %v", err)
				return
			}
			c.mu.Unlock()
			for _, fn := range c.update {
				fn(c)
			}
		default:
		}
	})
	viper.WatchConfig()

	return
}

func (c *Config) OnChange(fn func(*Config) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.update = append(c.update, fn)
}

func (c *Config) Listen() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.body.Listen
}

func (c *Config) Bootstrap() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.body.Bootstrap
}

func (c *Config) PublicIP() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.body.PublicIP
}

func (c *Config) PrivateKey() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.body.PrivateKey
}
