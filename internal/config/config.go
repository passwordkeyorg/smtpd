package config

import "os"

type Config struct {
	SMTPListen string
}

func Load() Config {
	return Config{SMTPListen: getenv("SMTP_LISTEN", ":2525")}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
