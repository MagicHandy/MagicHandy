import type { HandyBleSession } from "./handy-ble-session";

// The lowest round trip reduces the effect of a delayed Bluetooth/browser
// sample. Device time is uint32 milliseconds since boot, never UNIX time.
export async function syncHandyBleClock(session: HandyBleSession): Promise<void> {
  let best: { offset: number; rtd: number } | undefined;
  for (let index = 0; index < 3; index += 1) {
    let before = 0;
    let monotonicBefore = 0;
    const response = await session.request("clock/offset/get", () => {
      before = Date.now();
      monotonicBefore = performance.now();
      return {};
    });
    const after = Date.now();
    const rtd = performance.now() - monotonicBefore;
    const clock = response.clock_offset_get as { time?: number } | undefined;
    if (Number.isInteger(clock?.time) && clock!.time! >= 0 && clock!.time! <= 0xffffffff && Math.abs(after - before - rtd) < 100) {
      const sample = { offset: Math.round((before + after) / 2 - clock!.time!), rtd: Math.round(rtd) };
      if (!best || sample.rtd < best.rtd) best = sample;
    }
  }
  if (!best) throw new Error("Handy Bluetooth clock sync did not return usable samples.");
  await session.request("clock/offset/set", { clock_offset: best.offset, rtd: best.rtd }, { waitForResponse: false });
}
