package config

import "testing"

func TestSceneMotionDefaultAndUpdateRoundTrip(t *testing.T) {
	settings := DefaultSettings().LLM
	if !settings.SceneMotion() {
		t.Fatal("settings saved before the option must keep scene motion on")
	}
	update := LLMUpdateFromSettings(settings)
	enabled := false
	update.SceneMotionEnabled = &enabled
	next, err := applyLLMUpdate(settings, update)
	if err != nil || next.SceneMotion() {
		t.Fatal("switching scene motion off was not saved", err)
	}
	enabled = true
	if next.SceneMotion() {
		t.Fatal("write payload aliases saved setting")
	}
	update.SceneMotionEnabled = nil
	next, err = applyLLMUpdate(next, update)
	if err != nil || next.SceneMotion() {
		t.Fatal("an older client reset the saved setting", err)
	}
	full := DefaultSettings()
	full.LLM = next
	clone := cloneSettings(full)
	*clone.LLM.SceneMotionEnabled = true
	if full.LLM.SceneMotion() {
		t.Fatal("snapshot aliases saved setting")
	}
	if got := LLMUpdateFromSettings(next).SceneMotionEnabled; got == nil || *got {
		t.Fatal("complete update omitted the off value")
	}
}
