package myip

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// httpClient with timeout for reliability in cron jobs
var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

// GetPublicIP retrieves the public IP for current network
func GetPublicIP() (string, error) {
	resp, err := httpClient.Get("https://checkip.amazonaws.com")
	if err != nil {
		return "", fmt.Errorf("http get public ip error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read geoLocation stream: %w", err)
	}

	return strings.Trim(string(body), "\n"), nil
}

// geoLocation represents the response from ip-api.com for proxy detection.
type geoLocation struct {
	//query  string `json:"query"`
	//status string `json:"status"`
	Proxy bool `json:"proxy"`
}

// IsProxyIP checks whether public-ip actually is a proxy-public-ip, using geo location api
func IsProxyIP(ip *string) (bool, error) {
	if ip == nil {
		return false, fmt.Errorf("ip cannot be nil")
	}
	resp, err := httpClient.Get(fmt.Sprintf("https://ip-api.com/json/%s?fields=query,status,proxy", *ip))
	if err != nil {
		return false, fmt.Errorf("http check if-public-ip-is-proxy error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read geoLocation stream: %w", err)
	}

	location, err := toJSON(body)
	if err != nil {
		return false, err
	}

	return location.Proxy, nil
}

// toJSON unmarshals the JSON response into a geoLocation struct.
func toJSON(body []byte) (geoLocation, error) {
	var location geoLocation
	if err := json.Unmarshal(body, &location); err != nil {
		return location, fmt.Errorf("error decoding json geoLocation: %w", err)
	}
	return location, nil
}
