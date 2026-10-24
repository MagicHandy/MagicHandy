package transport

import "testing"

func TestSparseCloudPreferenceDoesNotChangeLinkedDeviceSampling(t *testing.T) {
	cloud := &CloudRESTTransport{}
	if !cloud.MotionSamplingCapabilities().PreferSparseGeneratedMotion {
		t.Fatal("single Cloud owner lost its fitting preference")
	}
	for _, companion := range []Transport{&CloudRESTTransport{}, NewFake()} {
		linked := NewFanOut(cloud, companion).(MotionSamplingCapabilitiesProvider)
		if linked.MotionSamplingCapabilities().PreferSparseGeneratedMotion {
			t.Fatal("applied a single-device preference to a linked session")
		}
	}
}
