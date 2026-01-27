package xmltv

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/patrickjmcd/hdhr2xmltv/internal/hdhr"
)

const (
	xmltvTimeFormat = "20060102150405 -0700"
	generatorName   = "hdhr2xmltv"
	generatorURL    = "https://github.com/patrickjmcd/hdhr2xmltv"
)

// Writer converts HDHomeRun guide data to XMLTV format.
type Writer struct {
	output io.Writer
}

// NewWriter creates a new XMLTV writer.
func NewWriter(output io.Writer) *Writer {
	return &Writer{output: output}
}

// Write converts HDHomeRun guide data to XMLTV and writes it to the output.
func (w *Writer) Write(guide []hdhr.GuideChannel) error {
	tv := TV{
		SourceInfoName:    "HDHomeRun",
		GeneratorInfoName: generatorName,
		GeneratorInfoURL:  generatorURL,
		Channels:          make([]Channel, 0, len(guide)),
		Programmes:        make([]Programme, 0),
	}

	for _, ch := range guide {
		channelID := sanitizeChannelID(ch.GuideNumber)

		xmlCh := Channel{
			ID: channelID,
			DisplayName: []CommonText{
				{Value: ch.GuideName},
				{Value: ch.GuideNumber},
			},
		}

		if ch.ImageURL != "" {
			xmlCh.Icon = &Icon{Src: ch.ImageURL}
		}

		tv.Channels = append(tv.Channels, xmlCh)

		for _, prog := range ch.Guide {
			xmlProg := convertProgramme(prog, channelID)
			tv.Programmes = append(tv.Programmes, xmlProg)
		}
	}

	if _, err := w.output.Write([]byte(xml.Header)); err != nil {
		return fmt.Errorf("failed to write XML header: %w", err)
	}

	encoder := xml.NewEncoder(w.output)
	encoder.Indent("", "  ")

	if err := encoder.Encode(tv); err != nil {
		return fmt.Errorf("failed to encode XMLTV: %w", err)
	}

	if _, err := w.output.Write([]byte("\n")); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	return nil
}

func convertProgramme(prog hdhr.GuideEntry, channelID string) Programme {
	startTime := time.Unix(prog.StartTime, 0)
	endTime := time.Unix(prog.EndTime, 0)

	xmlProg := Programme{
		Start:   startTime.Format(xmltvTimeFormat),
		Stop:    endTime.Format(xmltvTimeFormat),
		Channel: channelID,
		Title:   []CommonText{{Lang: "en", Value: prog.Title}},
	}

	if prog.EpisodeTitle != "" {
		xmlProg.SubTitle = []CommonText{{Lang: "en", Value: prog.EpisodeTitle}}
	}

	if prog.Synopsis != "" {
		xmlProg.Desc = []CommonText{{Lang: "en", Value: prog.Synopsis}}
	}

	if prog.ImageURL != "" {
		xmlProg.Icon = &Icon{Src: prog.ImageURL}
	}

	if prog.EpisodeNumber != "" {
		xmlProg.EpisodeNum = []EpisodeNum{
			{System: "onscreen", Value: prog.EpisodeNumber},
		}

		// Try to parse and add xmltv_ns format
		if xmltvNS := parseEpisodeNumber(prog.EpisodeNumber); xmltvNS != "" {
			xmlProg.EpisodeNum = append(xmlProg.EpisodeNum, EpisodeNum{
				System: "xmltv_ns",
				Value:  xmltvNS,
			})
		}
	}

	if prog.ProgramID != "" {
		xmlProg.EpisodeNum = append(xmlProg.EpisodeNum, EpisodeNum{
			System: "dd_progid",
			Value:  prog.ProgramID,
		})
	}

	if prog.OriginalAirdate > 0 {
		originalDate := time.Unix(prog.OriginalAirdate, 0)
		xmlProg.Date = originalDate.Format("20060102")

		if prog.StartTime != prog.OriginalAirdate {
			xmlProg.PreviouslyShown = &PreviouslyShown{
				Start: originalDate.Format(xmltvTimeFormat),
			}
		}
	}

	for _, filter := range prog.Filter {
		xmlProg.Category = append(xmlProg.Category, CommonText{
			Lang:  "en",
			Value: filter,
		})
	}

	return xmlProg
}

func sanitizeChannelID(guideNumber string) string {
	id := strings.ReplaceAll(guideNumber, ".", "_")
	id = strings.ReplaceAll(id, " ", "_")
	return fmt.Sprintf("%s.hdhr", id)
}

// parseEpisodeNumber attempts to parse episode numbers like "S01E05" into xmltv_ns format.
func parseEpisodeNumber(ep string) string {
	ep = strings.ToUpper(ep)

	var season, episode int
	n, _ := fmt.Sscanf(ep, "S%dE%d", &season, &episode)
	if n == 2 {
		// xmltv_ns format: season-1.episode-1.part (0-indexed)
		return fmt.Sprintf("%d.%d.", season-1, episode-1)
	}

	return ""
}
