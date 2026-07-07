package funscript

import "testing"

func TestRejectTooShortSegment(t *testing.T) {
	result := EvaluateSegmentRecord(BlockRecord{
		DurationMS: 800,
		Amplitude:  40,
		Actions: []StoredAction{
			{At: 0, Pos: 10},
			{At: 500, Pos: 50},
		},
	})
	if result.Approved {
		t.Fatal("expected rejection")
	}
	if result.RejectReason == "" {
		t.Fatal("expected reject reason")
	}
}

func TestFilterSegmentRecordsSplitsAcceptedAndRejected(t *testing.T) {
	records := []BlockRecord{
		{
			ID:         "ok-01",
			DurationMS: 5000,
			Amplitude:  30,
			Actions: []StoredAction{
				{At: 0, Pos: 10},
				{At: 1000, Pos: 80},
			},
			Zone:   "top",
			Speed:  "slow",
			Rhythm: "steady",
		},
		{
			ID:         "bad-01",
			DurationMS: 500,
			Amplitude:  30,
			Actions:    []StoredAction{{At: 0, Pos: 10}},
		},
	}
	accepted, rejected := FilterSegmentRecords(records)
	if len(accepted) != 1 || accepted[0].ID != "ok-01" {
		t.Fatalf("accepted = %+v, want ok-01", accepted)
	}
	if len(rejected) != 1 {
		t.Fatalf("rejected = %+v, want one rejected record", rejected)
	}
}
