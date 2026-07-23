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

// poseRank returns the rotation index for pose comparisons.
func poseRank(pose Pose) int {
	switch pose {
	case PoseHandjob:
		return 0
	case PoseOral:
		return 1
	case PoseCavalgando:
		return 2
	case PoseDeepthroat:
		return 3
	default:
		return 0
	}
}

// ResolvePose keeps the current pose when the model tries to downgrade in the cycle.
func ResolvePose(current, requested Pose) Pose {
	if current == "" {
		current = PoseHandjob
	}
	if requested == "" {
		requested = PoseHandjob
	}
	if poseRank(requested) < poseRank(current) {
		return current
	}
	return requested
}

// ApplyStamina applies drain/recover and rotates pose when stamina is depleted.
func ApplyStamina(stamina float64, intent Intent) (float64, Intent) {
	return applyStamina(stamina, intent, true)
}

// ApplyRoteiroStaminaCommit resolves pose for a roteiro block; stamina changes during playback ticks.
func ApplyRoteiroStaminaCommit(stamina float64, currentPose Pose, intent Intent) (float64, Intent) {
	if currentPose == "" {
		currentPose = PoseHandjob
	}
	intent.Posicao = ResolvePose(currentPose, intent.Posicao)
	return stamina, intent
}

// ApplyStaminaCommit resolves pose; stamina delta is applied via playback ticks or bridge drain.
func ApplyStaminaCommit(stamina float64, currentPose Pose, intent Intent) (float64, Intent) {
	if currentPose == "" {
		currentPose = PoseHandjob
	}
	intent.Posicao = ResolvePose(currentPose, intent.Posicao)
	next := ApplyProceduralStamina(stamina, intent, float64(intent.DuracaoSegundos))
	if next <= 0 {
		intent.Posicao = NextPose(currentPose)
		return 100, intent
	}
	return next, intent
}

// ApplyStaminaForBridge drains stamina for filler segments without recovery bonuses.
func ApplyStaminaForBridge(stamina float64, currentPose Pose, intent Intent) (float64, Intent) {
	if currentPose == "" {
		currentPose = PoseHandjob
	}
	intent.Posicao = ResolvePose(currentPose, intent.Posicao)
	next := ApplyDrain(stamina, intent)
	if next <= 0 {
		intent.Posicao = NextPose(currentPose)
		return 100, intent
	}
	return next, intent
}

func applyStamina(stamina float64, intent Intent, recover bool) (float64, Intent) {
	next := ApplyDrain(stamina, intent)
	if next <= 0 {
		intent.Posicao = NextPose(intent.Posicao)
		return 100, intent
	}
	if recover {
		next = ApplyRecover(next, intent)
	}
	return next, intent
}
