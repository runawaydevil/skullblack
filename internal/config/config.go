package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config holds runtime configuration from environment variables.
type Config struct {
	BaserowAPIURL  string
	BaserowTableID string
	BaserowToken   string
	SiteName       string
	SiteURL        string
	Port           string
}

// Load reads required environment variables and returns Config or an error.
func Load() (Config, error) {
	var missing []string
	get := func(key string) string {
		v := strings.TrimSpace(os.Getenv(key))
		if v == "" {
			missing = append(missing, key)
		}
		return v
	}

	cfg := Config{
		BaserowAPIURL:  strings.TrimRight(get("BASEROW_API_URL"), "/"),
		BaserowTableID: get("BASEROW_TABLE_ID"),
		BaserowToken:   get("BASEROW_TOKEN"),
		SiteName:       get("SITE_NAME"),
		SiteURL:        strings.TrimRight(get("SITE_URL"), "/"),
		Port:           get("PORT"),
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	if err := validateHTTPURL("SITE_URL", cfg.SiteURL); err != nil {
		return Config{}, err
	}
	if err := validateHTTPURL("BASEROW_API_URL", cfg.BaserowAPIURL); err != nil {
		return Config{}, err
	}
	if err := validatePort("PORT", cfg.Port); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func validateHTTPURL(key, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return fmt.Errorf("%s must be a valid URL", key)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s must use http or https", key)
	}
	if strings.TrimSpace(u.Host) == "" {
		return fmt.Errorf("%s must include a host", key)
	}
	return nil
}

func validatePort(key, raw string) error {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("%s must be numeric", key)
	}
	if n < 1 || n > 65535 {
		return fmt.Errorf("%s must be between 1 and 65535", key)
	}
	return nil
}
