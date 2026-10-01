package config

import "testing"

func TestFreestyleUpgradePreservesThePreviousMotionStyle(t *testing.T) {
	for _, feel := range []string{FreestyleFeelGentle, FreestyleFeelBalanced, FreestyleFeelIntense} {
		t.Run(feel, func(t *testing.T) {
			settings := DefaultSettings()
			settings.Motion.Style, settings.Freestyle = feel, FreestyleSettings{}
			next, err := NormalizeSettings(settings)
			if err != nil {
				t.Fatal(err)
			}
			if next.Freestyle.Feel != feel || next.Freestyle.Shape != FreestyleShapeSteady {
				t.Fatalf("upgraded preferences = %+v", next.Freestyle)
			}
		})
	}
}

func TestFreestyleCustomControlsSurviveUnrelatedSettingsUpdates(t *testing.T) {
	settings := DefaultSettings()
	settings.Freestyle.Feel, settings.Freestyle.PacePercent, settings.Freestyle.FocusPercent = FreestyleFeelCustom, 0, 100
	settings.Freestyle.Shape, settings.Freestyle.ShapeMinutes = FreestyleShapeCooldown, 1
	next, err := NormalizeSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	if next.Freestyle != settings.Freestyle {
		t.Fatalf("custom preferences changed: %+v", next.Freestyle)
	}
}
