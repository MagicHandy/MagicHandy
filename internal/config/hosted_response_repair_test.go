package config

import "testing"

func TestHostedResponseRepairDefaultAndUpdateRoundTrip(t *testing.T) {
	settings := DefaultSettings().LLM
	if !settings.HostedResponseRepair() {
		t.Fatal("legacy settings must default repair on")
	}
	update := LLMUpdateFromSettings(settings)
	enabled := false
	update.HostedResponseRepairEnabled = &enabled
	next, err := applyLLMUpdate(settings, update)
	if err != nil || next.HostedResponseRepair() {
		t.Fatal("disable was not saved", err)
	}
	enabled = true
	if next.HostedResponseRepair() {
		t.Fatal("write payload aliases saved setting")
	}
	update.HostedResponseRepairEnabled = nil
	next, err = applyLLMUpdate(next, update)
	if err != nil || next.HostedResponseRepair() {
		t.Fatal("older client reset saved setting", err)
	}
	full := DefaultSettings()
	full.LLM = next
	clone := cloneSettings(full)
	*clone.LLM.HostedResponseRepairEnabled = true
	if full.LLM.HostedResponseRepair() {
		t.Fatal("snapshot aliases saved setting")
	}
	if got := LLMUpdateFromSettings(next).HostedResponseRepairEnabled; got == nil || *got {
		t.Fatal("complete update omitted disabled value")
	}
}
