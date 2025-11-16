package config

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// Константы для значений по умолчанию
const (
	defaultServerAddress = ":8080"
	defaultBaseURL       = "http://localhost:8080"
)

// EnvGetter интерфейс для получения переменных окружения
type EnvGetter interface {
	Get(key string) string
}

// RealEnvGetter реализация для реальных переменных окружения
type RealEnvGetter struct{}

func (r RealEnvGetter) Get(key string) string {
	return os.Getenv(key)
}

type Config struct {
	ServerAddress   string
	BaseURL         string
	FileStoragePath string
	PostgreSQLDns   string
}

func getConfigValue(envGetter EnvGetter, envKey, flagValue string) string {
	// Получаем значения переменной окружения (имеет приоритет над флагом)
	if envValue := envGetter.Get(envKey); envValue != "" {
		return envValue
	}
	// Вернём либо значения флага, либо значение по умолчанию
	return flagValue
}

// LoadConfig загружает конфигурацию с использованием реальных переменных окружения
func LoadConfig() Config {
	return LoadConfigWithEnv(RealEnvGetter{})
}

// LoadConfigWithEnv загружает конфигурацию с использованием переданного EnvGetter
func LoadConfigWithEnv(envGetter EnvGetter) Config {
	// Определяем флаги командной строки
	serverAddressFlag := flag.String("a", defaultServerAddress, "Server address")
	baseURLFlag := flag.String("b", defaultBaseURL, "Base URL")
	fileStoragePathLFlag := flag.String("f", "", "File storage path")
	postgreSQLDnsFlag := flag.String("d", "", "File storage path")
	flag.Parse()

	cfg := Config{
		ServerAddress:   getConfigValue(envGetter, "SERVER_ADDRESS", *serverAddressFlag),
		BaseURL:         getConfigValue(envGetter, "BASE_URL", *baseURLFlag),
		FileStoragePath: getConfigValue(envGetter, "FILE_STORAGE_PATH", *fileStoragePathLFlag),
		PostgreSQLDns:   getConfigValue(envGetter, "DATABASE_DSN", *postgreSQLDnsFlag),
	}

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
