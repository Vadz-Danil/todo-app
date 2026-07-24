package repository

import (
	"testing"
	"time"
)

// The browser reports the viewer's zone via Intl, and older platforms still
// return IANA names that were renamed years ago. PostgreSQL builds that ship
// tzdata without the "backward" compatibility links reject those names
// outright, so a zone name must never be handed to SQL — bucketing happens
// here, against Go's own tzdata, which does carry the aliases.
func TestGroupCompletionsByLocalDay_AcceptsRenamedZones(t *testing.T) {
	// 2024-03-01T22:30:00Z is already 2024-03-02 in Kyiv (UTC+2).
	stamp := time.Date(2024, 3, 1, 22, 30, 0, 0, time.UTC)

	cases := []struct {
		name string
		zone string
	}{
		{"current name", "Europe/Kyiv"},
		{"renamed in tzdata 2022b", "Europe/Kiev"},
		{"renamed in tzdata 1993", "Asia/Calcutta"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loc, err := time.LoadLocation(tc.zone)
			if err != nil {
				t.Skipf("%s is absent from this platform's tzdata", tc.zone)
			}

			days := groupCompletionsByLocalDay([]time.Time{stamp}, loc)

			if len(days) != 1 {
				t.Fatalf("got %d buckets, want 1", len(days))
			}
			want := stamp.In(loc).Format("2006-01-02")
			if days[0].Date != want {
				t.Errorf("Date = %q, want %q", days[0].Date, want)
			}
			if days[0].Count != 1 {
				t.Errorf("Count = %d, want 1", days[0].Count)
			}
		})
	}
}

func TestGroupCompletionsByLocalDay_BucketsByTheViewersCalendarDay(t *testing.T) {
	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		t.Skip("Europe/Kyiv is absent from this platform's tzdata")
	}

	// Both stamps are 2024-06-10 in UTC, but 21:30Z is already the 11th in
	// Kyiv (UTC+3 in summer). Grouping in UTC would wrongly merge them.
	stamps := []time.Time{
		time.Date(2024, 6, 10, 8, 0, 0, 0, time.UTC),
		time.Date(2024, 6, 10, 21, 30, 0, 0, time.UTC),
	}

	days := groupCompletionsByLocalDay(stamps, kyiv)

	if len(days) != 2 {
		t.Fatalf("got %d buckets (%v), want 2 — the late stamp belongs to the next local day", len(days), days)
	}
	if days[0].Date != "2024-06-10" || days[1].Date != "2024-06-11" {
		t.Errorf("dates = %q, %q; want 2024-06-10, 2024-06-11", days[0].Date, days[1].Date)
	}
}

func TestGroupCompletionsByLocalDay_CountsAndOrders(t *testing.T) {
	days := groupCompletionsByLocalDay([]time.Time{
		time.Date(2024, 6, 12, 10, 0, 0, 0, time.UTC),
		time.Date(2024, 6, 10, 10, 0, 0, 0, time.UTC),
		time.Date(2024, 6, 12, 15, 0, 0, 0, time.UTC),
		time.Date(2024, 6, 11, 10, 0, 0, 0, time.UTC),
	}, time.UTC)

	want := []struct {
		date  string
		count int
	}{
		{"2024-06-10", 1},
		{"2024-06-11", 1},
		{"2024-06-12", 2},
	}

	if len(days) != len(want) {
		t.Fatalf("got %d buckets (%v), want %d", len(days), days, len(want))
	}
	for i, w := range want {
		if days[i].Date != w.date || days[i].Count != w.count {
			t.Errorf("bucket %d = {%s, %d}, want {%s, %d}", i, days[i].Date, days[i].Count, w.date, w.count)
		}
	}
}

func TestGroupCompletionsByLocalDay_EmptyInputYieldsEmptySlice(t *testing.T) {
	days := groupCompletionsByLocalDay(nil, time.UTC)
	if days == nil {
		t.Fatal("want an empty slice rather than nil, so callers can range over it")
	}
	if len(days) != 0 {
		t.Errorf("got %d buckets, want 0", len(days))
	}
}

func TestGroupCompletionsByLocalDay_NilLocationFallsBackToUTC(t *testing.T) {
	stamp := time.Date(2024, 6, 10, 8, 0, 0, 0, time.UTC)

	days := groupCompletionsByLocalDay([]time.Time{stamp}, nil)

	if len(days) != 1 || days[0].Date != "2024-06-10" {
		t.Fatalf("got %v, want a single 2024-06-10 bucket", days)
	}
}
