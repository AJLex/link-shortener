package config

import (
	"flag"
	"os"
	"testing"
)

// MockEnvGetter мок для тестирования
type MockEnvGetter struct {
	envVars map[string]string
}

func (m MockEnvGetter) Get(key string) string {
	return m.envVars[key]
}

func newMockEnvGetter() MockEnvGetter {
	return MockEnvGetter{
		envVars: make(map[string]string),
	}
}

func TestLoadConfigWithEnv(t *testing.T) {
	tests := []struct {
		name           string
		envVars        map[string]string
		args           []string
		expectedConfig Config
	}{
		{
			name:    "default values",
			envVars: map[string]string{},
			args:    []string{"test"},
			expectedConfig: Config{
				ServerAddress: ":8080",
				BaseURL:       "http://localhost:8080",
			},
		},
		{
			name: "environment variables override defaults",
			envVars: map[string]string{
				"SERVER_ADDRESS": ":9090",
				"BASE_URL":       "https://example.com",
			},
			args: []string{"test"},
			expectedConfig: Config{
				ServerAddress: ":9090",
				BaseURL:       "https://example.com",
			},
		},
		{
			name:    "command line flags override defaults",
			envVars: map[string]string{},
			args:    []string{"test", "-a", ":7070", "-b", "http://test.com"},
			expectedConfig: Config{
				ServerAddress: ":7070",
				BaseURL:       "http://test.com",
			},
		},
		{
			name: "environment variables have priority over flags",
			envVars: map[string]string{
				"SERVER_ADDRESS": ":9090",
				"BASE_URL":       "https://env.example.com",
			},
			args: []string{"test", "-a", ":7070", "-b", "http://flag.example.com"},
			expectedConfig: Config{
				ServerAddress: ":9090",
				BaseURL:       "https://env.example.com",
			},
		},
		{
			name: "base url normalization removes trailing slash",
			envVars: map[string]string{
				"BASE_URL": "https://example.com/",
			},
			args: []string{"test"},
			expectedConfig: Config{
				ServerAddress: ":8080",
				BaseURL:       "https://example.com",
			},
		},
		{
			name: "partial environment variables",
			envVars: map[string]string{
				"SERVER_ADDRESS": ":9090",
			},
			args: []string{"test"},
			expectedConfig: Config{
				ServerAddress: ":9090",
				BaseURL:       "http://localhost:8080",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Сбрасываем флаги перед каждым тестом
			flag.CommandLine = flag.NewFlagSet(tt.args[0], flag.ContinueOnError)
			os.Args = tt.args

			mockEnv := newMockEnvGetter()
			mockEnv.envVars = tt.envVars

			cfg := LoadConfigWithEnv(mockEnv)

			if cfg.ServerAddress != tt.expectedConfig.ServerAddress {
				t.Errorf("ServerAddress = %v, want %v", cfg.ServerAddress, tt.expectedConfig.ServerAddress)
			}

			if cfg.BaseURL != tt.expectedConfig.BaseURL {
				t.Errorf("BaseURL = %v, want %v", cfg.BaseURL, tt.expectedConfig.BaseURL)
			}

			// Проверяем валидацию
			if err := cfg.Validate(); err != nil {
				t.Errorf("Validate() returned error: %v", err)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	// Устанавливаем тестовые аргументы
	os.Args = []string{"test", "-a", ":9090", "-b", "http://test.com"}

	cfg := LoadConfig()

	expected := Config{
		ServerAddress: ":9090",
		BaseURL:       "http://test.com",
	}

	if cfg.ServerAddress != expected.ServerAddress {
		t.Errorf("LoadConfig() ServerAddress = %v, want %v", cfg.ServerAddress, expected.ServerAddress)
	}

	if cfg.BaseURL != expected.BaseURL {
		t.Errorf("LoadConfig() BaseURL = %v, want %v", cfg.BaseURL, expected.BaseURL)
	}
}

func TestGetConfigValue(t *testing.T) {
	mockEnv := newMockEnvGetter()

	tests := []struct {
		name       string
		envVars    map[string]string
		envKey     string
		flagValue  string
		wantResult string
	}{
		{
			name:       "environment variable has priority",
			envVars:    map[string]string{"TEST_KEY": "env_value"},
			envKey:     "TEST_KEY",
			flagValue:  "flag_value",
			wantResult: "env_value",
		},
		{
			name:       "flag value when environment variable is empty",
			envVars:    map[string]string{},
			envKey:     "TEST_KEY",
			flagValue:  "flag_value",
			wantResult: "flag_value",
		},
		{
			name:       "flag value when environment variable not set",
			envVars:    map[string]string{"OTHER_KEY": "other_value"},
			envKey:     "TEST_KEY",
			flagValue:  "flag_value",
			wantResult: "flag_value",
		},
		{
			name:       "empty flag value when no environment variable",
			envVars:    map[string]string{},
			envKey:     "TEST_KEY",
			flagValue:  "",
			wantResult: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockEnv.envVars = tt.envVars
			result := getConfigValue(mockEnv, tt.envKey, tt.flagValue)

			if result != tt.wantResult {
				t.Errorf("getConfigValue() = %v, want %v", result, tt.wantResult)
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				ServerAddress: ":8080",
				BaseURL:       "http://localhost:8080",
			},
			wantErr: false,
		},
		{
			name: "empty server address",
			config: Config{
				ServerAddress: "",
				BaseURL:       "http://localhost:8080",
			},
			wantErr: true,
		},
		{
			name: "empty base url",
			config: Config{
				ServerAddress: ":8080",
				BaseURL:       "",
			},
			wantErr: true,
		},
		{
			name: "both empty",
			config: Config{
				ServerAddress: "",
				BaseURL:       "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
