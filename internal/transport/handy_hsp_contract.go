package transport

import (
	"errors"
	"fmt"
	"math"
)

// Cloud and BLE reach the same firmware HSP buffer. Validate its protobuf
// limits before either owner sets up a stream or sends any points.
func validateHandyHSPPoints(points []TimedPoint) error {
	if len(points) == 0 {
		return errors.New("HSP add requires at least one point")
	}
	for index, point := range points {
		if math.IsNaN(point.PositionPercent) || math.IsInf(point.PositionPercent, 0) || point.PositionPercent < 0 || point.PositionPercent > 100 {
			return fmt.Errorf("HSP point %d x must be finite and between 0 and 100", index)
		}
		if point.TimeMillis < 0 || point.TimeMillis > int64(^uint32(0)) {
			return fmt.Errorf("HSP point %d t must fit an unsigned 32-bit millisecond timestamp", index)
		}
		if index > 0 && point.TimeMillis <= points[index-1].TimeMillis {
			return fmt.Errorf("HSP point %d t must be strictly increasing", index)
		}
	}
	return nil
}

func validateHandyHSPStart(start int64) error {
	if start < 0 || start > int64(1<<31-1) {
		return errors.New("HSP play start time must fit a non-negative signed 32-bit millisecond timestamp")
	}
	return nil
}
