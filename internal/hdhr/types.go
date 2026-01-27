package hdhr

// DeviceInfo represents the HDHomeRun device discovery response.
type DeviceInfo struct {
	FriendlyName    string `json:"FriendlyName"`
	ModelNumber     string `json:"ModelNumber"`
	FirmwareName    string `json:"FirmwareName"`
	FirmwareVersion string `json:"FirmwareVersion"`
	DeviceID        string `json:"DeviceID"`
	DeviceAuth      string `json:"DeviceAuth"`
	BaseURL         string `json:"BaseURL"`
	LineupURL       string `json:"LineupURL"`
}

// Channel represents a channel in the lineup.
type Channel struct {
	GuideNumber string `json:"GuideNumber"`
	GuideName   string `json:"GuideName"`
	VideoCodec  string `json:"VideoCodec"`
	AudioCodec  string `json:"AudioCodec"`
	HD          int    `json:"HD"`
	URL         string `json:"URL"`
}

// GuideEntry represents a program entry from the guide API.
type GuideEntry struct {
	StartTime       int64    `json:"StartTime"`
	EndTime         int64    `json:"EndTime"`
	Title           string   `json:"Title"`
	EpisodeNumber   string   `json:"EpisodeNumber"`
	EpisodeTitle    string   `json:"EpisodeTitle"`
	Synopsis        string   `json:"Synopsis"`
	ImageURL        string   `json:"ImageURL"`
	SeriesID        string   `json:"SeriesID"`
	ProgramID       string   `json:"ProgramID"`
	OriginalAirdate int64    `json:"OriginalAirdate"`
	Filter          []string `json:"Filter"`
}

// GuideChannel represents a channel with its guide data.
type GuideChannel struct {
	GuideNumber string       `json:"GuideNumber"`
	GuideName   string       `json:"GuideName"`
	Affiliate   string       `json:"Affiliate"`
	ImageURL    string       `json:"ImageURL"`
	Guide       []GuideEntry `json:"Guide"`
}
