package platform

import (
	"fmt"
	"net"
	"os"
)

type Config struct{ Address, Environment string }

func LoadConfig() (Config, error) {
	cfg := Config{Address: "127.0.0.1:8080", Environment: "development"}
	if value := os.Getenv("API_ADDR"); value != "" {
		cfg.Address = value
	}
	if value := os.Getenv("APP_ENV"); value != "" {
		cfg.Environment = value
	}
	if _, _, err := net.SplitHostPort(cfg.Address); err != nil {
		return Config{}, fmt.Errorf("invalid API_ADDR: %w", err)
	}
	if cfg.Environment != "development" && cfg.Environment != "production" {
		return Config{}, fmt.Errorf("APP_ENV must be development or production")
	}
	return cfg, nil
}
