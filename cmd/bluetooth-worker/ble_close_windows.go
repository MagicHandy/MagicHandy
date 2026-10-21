package main

import "tinygo.org/x/bluetooth"

// The pinned upstream fix closes discovered GATT services synchronously before
// the session and device. Process exit alone leaves Windows service ownership
// behind for several seconds and can deny the next worker's discovery. The
// upstream disconnect callback no longer calls Disconnect recursively.
func closeNativeDevice(device bluetooth.Device) error { return device.Disconnect() }
