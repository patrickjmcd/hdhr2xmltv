package xmltv

import "encoding/xml"

// TV is the root element of an XMLTV document.
type TV struct {
	XMLName           xml.Name   `xml:"tv"`
	SourceInfoURL     string     `xml:"source-info-url,attr,omitempty"`
	SourceInfoName    string     `xml:"source-info-name,attr,omitempty"`
	GeneratorInfoName string     `xml:"generator-info-name,attr,omitempty"`
	GeneratorInfoURL  string     `xml:"generator-info-url,attr,omitempty"`
	Channels          []Channel  `xml:"channel"`
	Programmes        []Programme `xml:"programme"`
}

// Channel represents a TV channel in XMLTV format.
type Channel struct {
	ID          string       `xml:"id,attr"`
	DisplayName []CommonText `xml:"display-name"`
	Icon        *Icon        `xml:"icon,omitempty"`
}

// Programme represents a TV program in XMLTV format.
type Programme struct {
	Start       string       `xml:"start,attr"`
	Stop        string       `xml:"stop,attr"`
	Channel     string       `xml:"channel,attr"`
	Title       []CommonText `xml:"title"`
	SubTitle    []CommonText `xml:"sub-title,omitempty"`
	Desc        []CommonText `xml:"desc,omitempty"`
	Date        string       `xml:"date,omitempty"`
	Category    []CommonText `xml:"category,omitempty"`
	EpisodeNum  []EpisodeNum `xml:"episode-num,omitempty"`
	Icon        *Icon        `xml:"icon,omitempty"`
	PreviouslyShown *PreviouslyShown `xml:"previously-shown,omitempty"`
}

// CommonText represents text with an optional language attribute.
type CommonText struct {
	Lang  string `xml:"lang,attr,omitempty"`
	Value string `xml:",chardata"`
}

// Icon represents an image/icon element.
type Icon struct {
	Src    string `xml:"src,attr"`
	Width  string `xml:"width,attr,omitempty"`
	Height string `xml:"height,attr,omitempty"`
}

// EpisodeNum represents an episode number in various formats.
type EpisodeNum struct {
	System string `xml:"system,attr,omitempty"`
	Value  string `xml:",chardata"`
}

// PreviouslyShown indicates a rerun.
type PreviouslyShown struct {
	Start string `xml:"start,attr,omitempty"`
}
