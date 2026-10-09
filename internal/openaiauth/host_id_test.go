package openaiauth

import (
	"strings"
	"testing"
)

func TestHostIDRepairsOnlyUnregisteredPrereleaseHosts(t *testing.T) {
	for _, registered := range []bool{false, true} {
		t.Run(map[bool]string{false: "unregistered", true: "registered"}[registered], func(t *testing.T) {
			directory := t.TempDir()
			manager, err := Open(Options{DataDir: directory})
			if err != nil {
				t.Fatal(err)
			}
			manager.mu.Lock()
			err = manager.transaction(t.Context(), func(data *credentialFile) error {
				data.HostID = "magichandy-prerelease-format"
				if registered {
					data.Profiles = []registration{{ID: "profile", ClientID: "issued", Subject: "verified"}}
				}
				return nil
			})
			manager.mu.Unlock()
			manager.Close()
			if err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(Options{DataDir: directory})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(reopened.Close)
			reopened.mu.Lock()
			defer reopened.mu.Unlock()
			if err := reopened.transaction(t.Context(), func(data *credentialFile) error {
				if registered && data.HostID != "magichandy-prerelease-format" {
					t.Fatal("changed registered host identity")
				}
				if !registered && !strings.HasPrefix(data.HostID, "urn:uuid:") {
					t.Fatal("unregistered host was not repaired")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
