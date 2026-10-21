package main

import "tinygo.org/x/bluetooth"

func closeNativeDevice(device bluetooth.Device) error { return device.Disconnect() }
