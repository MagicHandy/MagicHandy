//go:build windows || linux

package main

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"

	"tinygo.org/x/bluetooth"

	"github.com/mapledaemon/MagicHandy/internal/ble"
)

// Discover collects advertisements only. An unnamed manufacturer payload is a
// candidate, not a COYOTE signature: identity is checked by GATT after selection.
// The official app also uses private advertisements for previously bound units.
func (a *radioAdapter) Discover(ctx context.Context, profile ble.Profile) ([]ble.Candidate, error) {
	var mu sync.Mutex
	found := make(map[string]ble.Candidate)
	finished := make(chan error, 1)
	go func() {
		finished <- a.adapter.Scan(func(_ *bluetooth.Adapter, result bluetooth.ScanResult) {
			name := strings.TrimSpace(result.LocalName())
			if !matchesDiscovery(profile, name, result.ManufacturerData()) || result.RSSI >= 0 {
				return
			}
			address := normalizeAddress(result.Address.String())
			mu.Lock()
			defer mu.Unlock()
			previous, exists := found[address]
			if !exists && len(found) == 64 {
				weakest := ""
				for key, entry := range found {
					if weakest == "" || entry.RSSI < found[weakest].RSSI {
						weakest = key
					}
				}
				if int(result.RSSI) <= found[weakest].RSSI {
					return
				}
				delete(found, weakest)
			}
			if name == "" {
				name = previous.Name
			}
			rssi := int(result.RSSI)
			if exists {
				rssi = max(rssi, previous.RSSI)
			}
			found[address] = ble.Candidate{Address: address, Name: name, RSSI: rssi}
		})
	}()
	select {
	case err := <-finished:
		if err != nil {
			return nil, err
		}
	case <-ctx.Done():
		_ = a.adapter.StopScan()
		<-finished
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil, ctx.Err()
	}
	mu.Lock()
	defer mu.Unlock()
	candidates := make([]ble.Candidate, 0, len(found))
	for _, candidate := range found {
		candidates = append(candidates, candidate)
	}
	slices.SortFunc(candidates, func(a, b ble.Candidate) int { return b.RSSI - a.RSSI })
	return candidates, nil
}

func matchesDiscovery(profile ble.Profile, name string, manufacturers []bluetooth.ManufacturerDataElement) bool {
	if profile.MatchesName(name) {
		return true
	}
	if profile.ID != ble.ProfileCoyote || name != "" {
		return false
	}
	for _, manufacturer := range manufacturers {
		// Includes the two bytes exposed separately as CompanyID. The private
		// identifier is not a brand signature; retain only its candidate shape.
		length := len(manufacturer.Data) + 2
		if length >= 8 && length <= 16 {
			return true
		}
	}
	return false
}
