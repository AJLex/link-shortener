package config

import (
	"flag"
	"fmt"
	"strings"
)

type Config struct {
	ServerAddress string
	BaseURL       string
}

func LoadConfig() *Config {
	cfg := &Config{}

	// Определяем флаги командной строки
	flag.StringVar(&cfg.ServerAddress, "a", "localhost:8080", "HTTP server address")
	flag.StringVar(&cfg.BaseURL, "b", "http://localhost:8080", "Base URL for shortened links")

	flag.Parse()

	// Нормализуем BaseURL (убираем trailing slash)
	cfg.BaseURL = strings.TrimSuffix(cfg.BaseURL, "/")

	return cfg
}

// Вспомогательный метод для проверки конфигурации
func (c *Config) Validate() error {
	if c.ServerAddress == "" {
		return fmt.Errorf("server address cannot be empty")
	}
	if c.BaseURL == "" {
		return fmt.Errorf("base URL cannot be empty")
	}
	return nil
}
