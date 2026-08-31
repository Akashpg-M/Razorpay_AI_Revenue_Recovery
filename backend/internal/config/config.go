package config

import "os"

type Config struct {
	Environment        string
	Port               string
	DatabaseURL        string
	RedisURL           string
	DecisionServiceURL string
}

func Load() Config {
	return Config{
		Environment: getEnv("APP_ENV", "development"),
		Port:        getEnv("BACKEND_PORT", "8080"),

		DatabaseURL: getEnv(
			"DATABASE_URL",
			"postgres://recovery:recovery_dev@localhost:5432/revenue_recovery?sslmode=disable",
		),

		RedisURL: getEnv(
			"REDIS_URL",
			"redis://localhost:6379/0",
		),

		DecisionServiceURL: getEnv(
			"DECISION_SERVICE_URL",
			"http://localhost:8001",
		),
	}
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)

	if value == "" {
		return fallback
	}

	return value
}
