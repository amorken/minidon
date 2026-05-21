// Package config handles loading minidon's runtime configuration from
// environment variables and command-line flags.
//
// All settings follow 12-factor conventions: env vars take precedence over
// built-in defaults, and flags may override env vars for local development.
//
// TODO: define Config struct and Load() function.
package config
