// Package config holds small helpers shared by the ingest and chat commands.
package config

import "os"

// EnvOr returns the environment variable named key, or fallback if it is unset or empty.
func EnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
