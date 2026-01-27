package hdhr

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	guideAPIURL = "https://api.hdhomerun.com/api/guide"
)

// Client is an HDHomeRun device client.
type Client struct {
	deviceIP   string
	httpClient *http.Client
}

// NewClient creates a new HDHomeRun client.
func NewClient(deviceIP string) *Client {
	return &Client{
		deviceIP: deviceIP,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetDeviceInfo fetches device information from the HDHomeRun.
func (c *Client) GetDeviceInfo() (*DeviceInfo, error) {
	reqUrl := fmt.Sprintf("http://%s/discover.json", c.deviceIP)

	resp, err := c.httpClient.Get(reqUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch device info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var info DeviceInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode device info: %w", err)
	}

	return &info, nil
}

// GetLineup fetches the channel lineup from the HDHomeRun.
func (c *Client) GetLineup() ([]Channel, error) {
	lineupURL := fmt.Sprintf("http://%s/lineup.json", c.deviceIP)

	resp, err := c.httpClient.Get(lineupURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch lineup: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var lineup []Channel
	if err := json.NewDecoder(resp.Body).Decode(&lineup); err != nil {
		return nil, fmt.Errorf("failed to decode lineup: %w", err)
	}

	return lineup, nil
}

// GetGuide fetches the TV guide data from Silicon Dust's API.
func (c *Client) GetGuide(device *DeviceInfo) ([]GuideChannel, error) {
	guideURL := fmt.Sprintf("%s?DeviceAuth=%s", guideAPIURL, url.QueryEscape(device.DeviceAuth))

	req, err := http.NewRequest(http.MethodGet, guideURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set User-Agent to match the HDHomeRun device
	userAgent := fmt.Sprintf("%s/%s (%s; %s)",
		device.FirmwareName,
		device.FirmwareVersion,
		device.ModelNumber,
		device.DeviceID,
	)
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch guide: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("guide API error (status %d): %s", resp.StatusCode, string(body))
	}

	var guide []GuideChannel
	if err := json.NewDecoder(resp.Body).Decode(&guide); err != nil {
		return nil, fmt.Errorf("failed to decode guide: %w", err)
	}

	return guide, nil
}
