package hdhr

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestGetGuidePagesUntilHorizon(t *testing.T) {
	// Each page returns 3 hours of programmes from Start (or 1000 for the
	// first page), overlapping the previous page by one programme. Requests
	// past 1000+4*3600 get a 400, as the real API does past its horizon.
	var starts []int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := int64(1000)
		if s := r.URL.Query().Get("Start"); s != "" {
			start, _ = strconv.ParseInt(s, 10, 64)
			start -= 3600 // overlap one programme
		}
		starts = append(starts, start)
		if start > 1000+4*3600 {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		var progs []GuideEntry
		for i := int64(0); i < 3; i++ {
			progs = append(progs, GuideEntry{StartTime: start + i*3600, EndTime: start + (i+1)*3600})
		}
		_ = json.NewEncoder(w).Encode([]GuideChannel{{GuideNumber: "5.1", Guide: progs}})
	}))
	defer srv.Close()

	c := NewClient("unused")
	c.guideAPIURL = srv.URL
	guide, err := c.GetGuide(&DeviceInfo{DeviceAuth: "x"})
	if err != nil {
		t.Fatalf("GetGuide: %v", err)
	}

	if len(guide) != 1 {
		t.Fatalf("want 1 channel, got %d", len(guide))
	}
	progs := guide[0].Guide
	for i := 1; i < len(progs); i++ {
		if progs[i].StartTime <= progs[i-1].StartTime {
			t.Fatalf("programmes not sorted/deduplicated at %d: %v", i, progs)
		}
	}
	if got := latestEndTime(guide); got <= 1000+3*3600 {
		t.Errorf("guide did not page past the first response: latest end %d", got)
	}
	if len(starts) < 3 {
		t.Errorf("expected several page requests, got %d", len(starts))
	}
}

func TestGetGuideFirstPageErrorIsReturned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient("unused")
	c.guideAPIURL = srv.URL
	if _, err := c.GetGuide(&DeviceInfo{DeviceAuth: "x"}); err == nil {
		t.Fatal("expected an error when the first page fails")
	}
}

func TestMergeGuideAddsNewChannels(t *testing.T) {
	a := []GuideChannel{{GuideNumber: "4.1", Guide: []GuideEntry{{StartTime: 1, EndTime: 2}}}}
	b := []GuideChannel{
		{GuideNumber: "4.1", Guide: []GuideEntry{{StartTime: 1, EndTime: 2}, {StartTime: 2, EndTime: 3}}},
		{GuideNumber: "9.1", Guide: []GuideEntry{{StartTime: 2, EndTime: 3}}},
	}
	got := mergeGuide(a, b)
	if len(got) != 2 || len(got[0].Guide) != 2 || len(got[1].Guide) != 1 {
		t.Fatalf("unexpected merge result: %+v", got)
	}
}
