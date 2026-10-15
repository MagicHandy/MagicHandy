// Package companion holds what every companion output shares when it follows
// the shared motion frame instead of moving a stroker: a bounded read-only
// copy of the frame, the clock that maps stream time to local time, and the
// coverage rule that turns a frame that ran out into silence and a reported
// starvation. Device encodings (Coyote pulse packets, vibration levels) stay
// with their owners; nothing here renders output or reshapes the frame.
package companion
