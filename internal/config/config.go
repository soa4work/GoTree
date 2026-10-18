// Package config resolves the runtime settings from flags and environment.
package config

import (
	"errors"
	"flag"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds everything the server needs to start.
type Config struct {
	// DataDir holds the SQLite database and uploaded media.
	DataDir string
	// ListenAddr is the address the HTTP server binds to, e.g. ":8080".
	ListenAddr string
	// AdminUser and AdminPassword create the first account on startup when
	// there is none yet, instead of the setup screen. Ignored afterwards.
	AdminUser     string
	AdminPassword string
	// MaxUploadMB limits the size of one uploaded file.
	MaxUploadMB int
	// TrustedProxies are the reverse proxies whose X-Forwarded-For header
	// names the client (GOTREE_TRUSTED_PROXIES, comma-separated addresses or
	// CIDR ranges). Empty: the TCP peer is the client.
	TrustedProxies []netip.Prefix
}

// MediaDir is where uploaded files are kept inside DataDir.
func (c Config) MediaDir() string {
	return filepath.Join(c.DataDir, "media")
}

// DBPath is the location of the SQLite database inside DataDir.
func (c Config) DBPath() string {
	return filepath.Join(c.DataDir, "gotree.db")
}

// Load parses args (without the program name). Flags win over environment
// variables, which win over the defaults.
func Load(args []string, getenv func(string) string) (Config, error) {
	cfg := Config{
		DataDir:       envOr(getenv, "GOTREE_DATA_DIR", "data"),
		ListenAddr:    envOr(getenv, "GOTREE_LISTEN_ADDR", ":8080"),
		AdminUser:     envOr(getenv, "GOTREE_ADMIN_USER", "admin"),
		AdminPassword: getenv("GOTREE_ADMIN_PASSWORD"),
		MaxUploadMB:   100,
	}
	if v := getenv("GOTREE_MAX_UPLOAD_MB"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, errors.New("GOTREE_MAX_UPLOAD_MB must be a positive number")
		}
		cfg.MaxUploadMB = n
	}
	proxies, err := parseProxies(getenv("GOTREE_TRUSTED_PROXIES"))
	if err != nil {
		return Config{}, err
	}
	cfg.TrustedProxies = proxies

	fs := flag.NewFlagSet("gotree", flag.ContinueOnError)
	fs.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "directory for the database and media (env GOTREE_DATA_DIR)")
	fs.StringVar(&cfg.ListenAddr, "listen", cfg.ListenAddr, "HTTP listen address (env GOTREE_LISTEN_ADDR)")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	if cfg.DataDir == "" {
		return Config{}, errors.New("data dir must not be empty")
	}
	if cfg.ListenAddr == "" {
		return Config{}, errors.New("listen address must not be empty")
	}
	return cfg, nil
}

// FromOS loads the configuration from the process arguments and environment.
func FromOS() (Config, error) {
	return Load(os.Args[1:], os.Getenv)
}

func envOr(getenv func(string) string, key, fallback string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseProxies reads a comma-separated list of IP addresses and CIDR ranges.
func parseProxies(v string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, entry := range strings.Split(v, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		p, err := netip.ParsePrefix(entry)
		if err != nil {
			a, aerr := netip.ParseAddr(entry)
			if aerr != nil {
				return nil, errors.New("GOTREE_TRUSTED_PROXIES must be a comma-separated list of IP addresses or CIDR ranges")
			}
			a = a.Unmap()
			p = netip.PrefixFrom(a, a.BitLen())
		}
		out = append(out, p.Masked())
	}
	return out, nil
}
