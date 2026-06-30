// Package config defines startup defaults, versioned settings, and durable
// settings storage.
package config

const defaultAddress = "127.0.0.1:49717"

// Config is the top-level application configuration.
type Config struct {
	Server Server
}

// Server contains local HTTP server settings.
type Server struct {
	Address string
}

// Default returns the built-in configuration used when no settings file exists.
func Default() Config {
	return Config{
		Server: Server{
			Address: defaultAddress,
		},
	}
}
