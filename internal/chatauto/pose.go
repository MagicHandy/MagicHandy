package chatauto

// NextPose advances to the next pose in the rotation cycle.
func NextPose(current Pose) Pose {
	order := []Pose{PoseHandjob, PoseOral, PoseCavalgando, PoseDeepthroat}
	for i, pose := range order {
		if pose == current {
			return order[(i+1)%len(order)]
		}
	}
	return PoseHandjob
}

// ApplyStamina applies drain/recover and rotates pose when stamina is depleted.
func ApplyStamina(stamina float64, intent Intent) (float64, Intent) {
	next := ApplyDrain(stamina, intent)
	if next <= 0 {
		intent.Posicao = NextPose(intent.Posicao)
		return 100, intent
	}
	next = ApplyRecover(next, intent)
	return next, intent
}
