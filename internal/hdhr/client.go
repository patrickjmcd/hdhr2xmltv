package hdhr

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"
)

const (
	guideAPIURL = "https://api.hdhomerun.com/api/guide"

	// maxGuidePages bounds how many times GetGuide pages forward. Each page
	// covers roughly 6-14 hours; SiliconDust serves about two days in total.
	maxGuidePages = 12
)

// Client is an HDHomeRun device client.
type Client struct {
	deviceIP    string
	guideAPIURL string
	httpClient  *http.Client
}

// NewClient creates a new HDHomeRun client.
func NewClient(deviceIP string) *Client {
	return &Client{
		deviceIP:    deviceIP,
		guideAPIURL: guideAPIURL,
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
//
// A single guide request only covers the next few hours, so GetGuide pages
// forward with the Start parameter from the latest end time seen until the
// API stops returning newer programmes (it answers 400 past its horizon).
func (c *Client) GetGuide(device *DeviceInfo) ([]GuideChannel, error) {
	guide, err := c.fetchGuidePage(device, 0)
	if err != nil {
		return nil, err
	}

	horizon := latestEndTime(guide)
	for page := 1; page < maxGuidePages && horizon > 0; page++ {
		next, err := c.fetchGuidePage(device, horizon)
		if err != nil {
			// The first page succeeded; a later failure (usually a 400 past the
			// API's horizon) just ends the guide early.
			break
		}
		guide = mergeGuide(guide, next)
		newHorizon := latestEndTime(guide)
		if newHorizon <= horizon {
			break
		}
		horizon = newHorizon
	}

	return guide, nil
}

// fetchGuidePage fetches one page of guide data. A zero start omits the Start
// parameter, which returns the guide from the current time.
func (c *Client) fetchGuidePage(device *DeviceInfo, start int64) ([]GuideChannel, error) {
	guideURL := fmt.Sprintf("%s?DeviceAuth=%s", c.guideAPIURL, url.QueryEscape(device.DeviceAuth))
	if start > 0 {
		guideURL += fmt.Sprintf("&Start=%d", start)
	}

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

// latestEndTime returns the latest programme end time in the guide.
func latestEndTime(guide []GuideChannel) int64 {
	var latest int64
	for _, ch := range guide {
		for _, prog := range ch.Guide {
			if prog.EndTime > latest {
				latest = prog.EndTime
			}
		}
	}
	return latest
}

// mergeGuide adds the programmes in next to guide, matching channels by
// GuideNumber. Programmes already present (same start time) are skipped, and
// each channel's programmes are kept sorted by start time.
func mergeGuide(guide, next []GuideChannel) []GuideChannel {
	index := make(map[string]int, len(guide))
	for i, ch := range guide {
		index[ch.GuideNumber] = i
	}

	for _, ch := range next {
		i, ok := index[ch.GuideNumber]
		if !ok {
			index[ch.GuideNumber] = len(guide)
			guide = append(guide, ch)
			continue
		}

		seen := make(map[int64]bool, len(guide[i].Guide))
		for _, prog := range guide[i].Guide {
			seen[prog.StartTime] = true
		}
		for _, prog := range ch.Guide {
			if !seen[prog.StartTime] {
				guide[i].Guide = append(guide[i].Guide, prog)
				seen[prog.StartTime] = true
			}
		}
		sort.SliceStable(guide[i].Guide, func(a, b int) bool {
			return guide[i].Guide[a].StartTime < guide[i].Guide[b].StartTime
		})
	}

	return guide
}
