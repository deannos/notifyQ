package config

import (
	"os"
	"strconv"
)

type Config struct {
	ListenAddr          string
	DatabasePath        string
	JWTSecret           string
	DefaultAdminUser    string
	DefaultAdminPass    string
	AllowRegistration   bool
	JWTExpiryHours      int
}

func Load() *Config {
	return &Config{
		ListenAddr:        getEnv("LISTEN_ADDR", ":8080"),
		DatabasePath:      getEnv("DATABASE_PATH", "notifications.db"),
		JWTSecret:         getEnv("JWT_SECRET", "change-me-in-production-please"),
		DefaultAdminUser:  getEnv("DEFAULT_ADMIN_USER", "admin"),
		DefaultAdminPass:  getEnv("DEFAULT_ADMIN_PASS", "admin"),
		AllowRegistration: getBoolEnv("ALLOW_REGISTRATION", true),
		JWTExpiryHours:    getIntEnv("JWT_EXPIRY_HOURS", 24),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getBoolEnv(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func getIntEnv(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}
