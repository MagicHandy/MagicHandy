//go:build windows || linux

package main

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRadioCloseReleasesLostConnectionExactlyOnce(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(map[bool]string{false: "connected", true: "link lost"}[lost], func(t *testing.T) {
			var calls atomic.Int32
			p := &radioPeripheral{
				adapter: &radioAdapter{lost: map[string]func(error){"selected": func(error) {}}},
				address: "selected", lost: lost,
				disconnect: func() error { calls.Add(1); return nil },
			}
			var group sync.WaitGroup
			for range 8 {
				group.Go(func() {
					if err := p.Close(); err != nil {
						t.Error(err)
					}
				})
			}
			group.Wait()
			if calls.Load() != 1 || len(p.adapter.lost) != 0 {
				t.Fatalf("native disposal calls=%d; callbacks=%d", calls.Load(), len(p.adapter.lost))
			}
			if err := p.Write([]byte{1}, false); err == nil {
				t.Fatal("write admitted after disposal")
			}
		})
	}
}

func TestRadioCloseReportsNativeFailureAndStillFencesWrites(t *testing.T) {
	failure := errors.New("native close failed")
	p := &radioPeripheral{adapter: &radioAdapter{lost: map[string]func(error){}}, disconnect: func() error { return failure }}
	if err := p.Close(); !errors.Is(err, failure) {
		t.Fatalf("close error = %v", err)
	}
	if err := p.Write([]byte{1}, false); err == nil {
		t.Fatal("write admitted after failed close")
	}
}
