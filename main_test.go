package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestQuietWindowContains(t *testing.T) {
	at := func(hhmm string) time.Time {
		tm, err := time.Parse("15:04", hhmm)
		if err != nil {
			t.Fatal(err)
		}
		return tm
	}
	tests := []struct {
		window string
		at     string
		want   bool
	}{
		{"", "01:00", false},
		{"00:55-02:00", "00:54", false},
		{"00:55-02:00", "00:55", true},
		{"00:55-02:00", "01:59", true},
		{"00:55-02:00", "02:00", false},
		{"23:30-01:00", "23:29", false},
		{"23:30-01:00", "23:30", true},
		{"23:30-01:00", "00:30", true},
		{"23:30-01:00", "01:00", false},
	}
	for _, tt := range tests {
		w, err := parseQuietWindow(tt.window)
		if err != nil {
			t.Fatalf("parseQuietWindow(%q): %v", tt.window, err)
		}
		if got := w.contains(at(tt.at)); got != tt.want {
			t.Errorf("window %q at %s = %v, want %v", tt.window, tt.at, got, tt.want)
		}
	}
}

func TestParseQuietWindowRejectsInvalid(t *testing.T) {
	for _, s := range []string{"01:00", "0100-0200", "25:00-01:00", "01:00-", "-01:00"} {
		if _, err := parseQuietWindow(s); err == nil {
			t.Errorf("parseQuietWindow(%q) accepted", s)
		}
	}
}

func TestResourceHealthy(t *testing.T) {
	tests := []struct {
		name    string
		targets string
		want    bool
	}{
		{"all healthy", `[{"healthStatus":"healthy"},{"healthStatus":"healthy"}]`, true},
		{"one unhealthy", `[{"healthStatus":"healthy"},{"healthStatus":"unhealthy"}]`, false},
		{"unknown", `[{"healthStatus":"unknown"}]`, false},
		{"field missing", `[{"targetId":1}]`, false},
		{"no targets", `[]`, false},
	}
	for _, tt := range tests {
		var r resource
		if err := json.Unmarshal([]byte(`{"fullDomain":"a.example.com","targets":`+tt.targets+`}`), &r); err != nil {
			t.Fatal(err)
		}
		if got := r.healthy(); got != tt.want {
			t.Errorf("%s: healthy() = %v, want %v", tt.name, got, tt.want)
		}
	}
}
