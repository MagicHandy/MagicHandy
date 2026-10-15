package main

import "tinygo.org/x/bluetooth"

// tinygo/bluetooth v0.16.0 calls Device.Disconnect from its WinRT disconnect
// callback, while explicit Disconnect closes the session before unregistering
// that callback. Calling it here can race the callback and release COM handles
// twice. Each worker serves exactly one connection attempt, so Windows handle
// cleanup belongs to process exit instead. The core waits for that exit before
// returning from link.Close, and kills a stalled worker. The server has already
// fenced writes and attempted the last-will Stop before this function runs.
// Keep this workaround in the worker; no native handle reaches the core.
func closeNativeDevice(_ bluetooth.Device) error { return nil }
