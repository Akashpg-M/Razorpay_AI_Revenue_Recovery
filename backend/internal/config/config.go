package config

import "os"

type Config struct {
	Environment           string
	Port                  string
	DatabaseURL           string
	RedisURL              string
	DecisionServiceURL    string
	RazorpayKeyID         string
	RazorpayKeySecret     string
	RazorpayWebhookSecret string
	RazorpayAPIURL        string
	EvaluationResultsPath string
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
		RazorpayKeyID:         getEnv("RAZORPAY_KEY_ID", ""),
		RazorpayKeySecret:     getEnv("RAZORPAY_KEY_SECRET", ""),
		RazorpayWebhookSecret: getEnv("RAZORPAY_WEBHOOK_SECRET", ""),
		RazorpayAPIURL:        getEnv("RAZORPAY_API_URL", "https://api.razorpay.com"),
		EvaluationResultsPath: getEnv("EVALUATION_RESULTS_PATH", "../decision-service/evaluation/results"),
	}
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)

	if value == "" {
		return fallback
	}

	return value
}
