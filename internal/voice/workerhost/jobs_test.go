package workerhost

import "testing"

func TestCancellationIncludesQueuedWorkAndDoesNotPoisonNewJobs(t *testing.T) {
	var jobs Jobs
	if !jobs.Track("active") || !jobs.Track("queued") {
		t.Fatal("admission failed")
	}
	active := jobs.Context("active")
	jobs.CancelAll()
	if active.Err() == nil || jobs.Context("queued").Err() == nil {
		t.Fatal("unload missed queued or active work")
	}
	jobs.Finish("active")
	jobs.Finish("queued")
	jobs.Cancel("future")
	if !jobs.Track("future") || jobs.Context("future").Err() != nil {
		t.Fatal("unknown cancel poisoned future work")
	}
	jobs.Finish("future")
}

func TestJobsAdmissionIsBoundedAndRejectsDuplicateIDs(t *testing.T) {
	var jobs Jobs
	for index := range 9 {
		if !jobs.Track(string(rune('a' + index))) {
			t.Fatal("bounded admission rejected valid slot")
		}
	}
	if jobs.Track("overflow") || jobs.Track("a") {
		t.Fatal("overflow or duplicate accepted")
	}
	jobs.CancelAll()
	for index := range 9 {
		jobs.Finish(string(rune('a' + index)))
	}
	if !jobs.Track("new") {
		t.Fatal("finished jobs retained their slots")
	}
	jobs.Finish("new")
}
