// Handy BLE protobuf codec used by the browser Bluetooth bridge.

const bluetoothDecoder = new TextDecoder();
const WIRE_VARINT = 0;
const WIRE_LENGTH = 2;
const WIRE_FIXED32 = 5;
const MESSAGE_TYPE_REQUEST = 1;
const MESSAGE_TYPE_RESPONSE = 3;
const MESSAGE_TYPE_NOTIFICATION = 4;
const HSP_POINT_PROTOCOL_MAX = 1000;
const REQUEST_FIELDS: Record<string, number> = {
  "clock/offset/get": 712,
  "clock/offset/set": 709,
  "slider/stroke": 841,
  "hsp/setup": 860,
  "hsp/add": 861,
  "hsp/play": 863,
  "hsp/stop": 864,
  "hsp/state": 867,
};
const HSP_RESPONSE_FIELDS = new Set([860, 861, 862, 863, 864, 865, 866, 867, 868, 869, 870, 871, 872]);
const HSP_NOTIFICATION_FIELDS = new Set([860, 861, 862, 863, 864, 865]);

interface Field {
  field: number;
  wire: number;
  value: bigint | Uint8Array | number;
}

function concatBytes(parts: Uint8Array[]): Uint8Array {
  const total = parts.reduce((sum, part) => sum + part.length, 0);
  const output = new Uint8Array(total);
  let offset = 0;
  for (const part of parts) {
    output.set(part, offset);
    offset += part.length;
  }
  return output;
}

function encodeVarint(value: number | bigint): Uint8Array {
  let next = typeof value === "bigint" ? value : BigInt(Math.max(0, Number(value) || 0));
  const bytes: number[] = [];
  while (next > 0x7fn) {
    bytes.push(Number((next & 0x7fn) | 0x80n));
    next >>= 7n;
  }
  bytes.push(Number(next));
  return Uint8Array.from(bytes);
}

function encodeSignedVarint(value: unknown): Uint8Array {
  let next = BigInt(Math.trunc(Number(value) || 0));
  if (next < 0) next = BigInt.asUintN(64, next);
  return encodeVarint(next);
}

function encodeZigZag64(value: unknown): Uint8Array {
  const n = BigInt(Math.trunc(Number(value) || 0));
  return encodeVarint((n << 1n) ^ (n >> 63n));
}

function fieldKey(field: number, wireType: number): Uint8Array {
  return encodeVarint((BigInt(field) << 3n) | BigInt(wireType));
}

function uintField(field: number, value: unknown): Uint8Array {
  return concatBytes([fieldKey(field, WIRE_VARINT), encodeVarint(Number(value) || 0)]);
}

function intField(field: number, value: unknown): Uint8Array {
  return concatBytes([fieldKey(field, WIRE_VARINT), encodeSignedVarint(value)]);
}

function sint64Field(field: number, value: unknown): Uint8Array {
  return concatBytes([fieldKey(field, WIRE_VARINT), encodeZigZag64(value)]);
}

function boolField(field: number, value: unknown): Uint8Array {
  return uintField(field, value ? 1 : 0);
}

function floatField(field: number, value: unknown): Uint8Array {
  const bytes = new Uint8Array(4);
  new DataView(bytes.buffer).setFloat32(0, Number(value) || 0, true);
  return concatBytes([fieldKey(field, WIRE_FIXED32), bytes]);
}

function lengthField(field: number, bytes: Uint8Array): Uint8Array {
  return concatBytes([fieldKey(field, WIRE_LENGTH), encodeVarint(bytes.length), bytes]);
}

function strokePercentValue(value: unknown, fallback: number): number {
  const number = Number(value);
  const safe = Number.isFinite(number) ? number : fallback;
  const percent = safe >= 0 && safe <= 1 ? safe * 100 : safe;
  return Math.max(0, Math.min(100, percent));
}

function bodyNumber(body: Record<string, unknown>, key: string, fallback: number): number {
  const value = Number(body[key]);
  return Number.isFinite(value) ? value : fallback;
}

function pointMessage(point: Record<string, unknown> = {}): Uint8Array {
  const x = point.x ?? point.pos ?? 50;
  const normalized = Math.max(0, Math.min(100, Number(x) || 0));
  return concatBytes([
    uintField(1, Math.max(0, Math.round(bodyNumber(point, "t", 0)))),
    uintField(2, Math.round((normalized / 100) * HSP_POINT_PROTOCOL_MAX)),
  ]);
}

function requestBodyForPath(path: string, body: Record<string, unknown> = {}): Uint8Array {
  if (path === "slider/stroke") {
    return concatBytes([
      floatField(1, strokePercentValue(body.min, 0)),
      floatField(2, strokePercentValue(body.max, 100)),
    ]);
  }
  if (path === "hsp/setup") return uintField(1, body.stream_id ?? 0);
  if (path === "hsp/add") {
    const points = Array.isArray(body.points) ? (body.points as Record<string, unknown>[]) : [];
    return concatBytes([
      ...points.map((point) => lengthField(1, pointMessage(point))),
      boolField(2, Boolean(body.flush)),
    ]);
  }
  if (path === "hsp/play") {
    return concatBytes([
      intField(1, body.start_time ?? 0),
      uintField(2, body.server_time ?? Date.now()),
      floatField(3, body.playback_rate ?? 1),
      boolField(4, Boolean(body.loop)),
      boolField(5, Boolean(body.pause_on_starving)),
    ]);
  }
  if (path === "clock/offset/set") {
    return concatBytes([
      sint64Field(1, body.clock_offset ?? 0),
      intField(2, body.rtd ?? 0),
    ]);
  }
  if (path === "hsp/stop" || path === "hsp/state" || path === "clock/offset/get") return new Uint8Array();
  throw new Error(`Bluetooth command is not implemented: ${path}`);
}

export function encodeHandyRequest(path: string, body: Record<string, unknown> = {}, id = 1): Uint8Array {
  const field = REQUEST_FIELDS[path];
  if (!field) throw new Error(`Bluetooth command is not implemented: ${path}`);
  const payload = requestBodyForPath(path, body);
  const request = concatBytes([lengthField(field, payload), uintField(2, id)]);
  return concatBytes([uintField(1, MESSAGE_TYPE_REQUEST), lengthField(2, request)]);
}

function readVarint(bytes: Uint8Array, offset: number): { value: bigint; offset: number } {
  let shift = 0n;
  let value = 0n;
  let index = offset;
  while (index < bytes.length) {
    const byte = BigInt(bytes[index]);
    value |= (byte & 0x7fn) << shift;
    index += 1;
    if ((byte & 0x80n) === 0n) return { value, offset: index };
    shift += 7n;
  }
  throw new Error("Truncated varint");
}

function toNumber(value: bigint): number {
  const maxSafe = BigInt(Number.MAX_SAFE_INTEGER);
  return Number(value > maxSafe ? maxSafe : value);
}

function toSigned32(value: bigint): number {
  const unsigned = Number(value & 0xffffffffn);
  return unsigned >= 0x80000000 ? unsigned - 0x100000000 : unsigned;
}

function zigZagToNumber(value: bigint): number {
  const decoded = (value >> 1n) ^ (-(value & 1n));
  return Number(decoded);
}

function parseFields(bytes: Uint8Array): Field[] {
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const fields: Field[] = [];
  let offset = 0;
  while (offset < bytes.length) {
    const key = readVarint(bytes, offset);
    offset = key.offset;
    const field = Number(key.value >> 3n);
    const wire = Number(key.value & 0x7n);
    let value: Field["value"];
    if (wire === WIRE_VARINT) {
      const parsed = readVarint(bytes, offset);
      value = parsed.value;
      offset = parsed.offset;
    } else if (wire === WIRE_LENGTH) {
      const length = readVarint(bytes, offset);
      offset = length.offset;
      const size = toNumber(length.value);
      value = bytes.slice(offset, offset + size);
      offset += size;
    } else if (wire === WIRE_FIXED32) {
      value = view.getFloat32(offset, true);
      offset += 4;
    } else {
      throw new Error(`Unsupported protobuf wire type ${wire}`);
    }
    fields.push({ field, wire, value });
  }
  return fields;
}

function firstField(fields: Field[], number: number): Field | undefined {
  return fields.find((item) => item.field === number);
}

function bytesField(item: Field | undefined): Uint8Array | undefined {
  return item && item.wire === WIRE_LENGTH && item.value instanceof Uint8Array ? item.value : undefined;
}

function varintField(item: Field | undefined): bigint | undefined {
  return item && item.wire === WIRE_VARINT && typeof item.value === "bigint" ? item.value : undefined;
}

function stringFromField(item: Field | undefined): string {
  const bytes = bytesField(item);
  return bytes ? bluetoothDecoder.decode(bytes) : "";
}

function hspPlayStateName(value: bigint | number): string {
  return {
    0: "not_initialized",
    1: "playing",
    2: "stopped",
    3: "paused",
    4: "starving",
  }[Number(value)] || String(value);
}

function parseHSPState(bytes: Uint8Array): Record<string, unknown> {
  const fields = parseFields(bytes);
  const readInt = (number: number) => {
    const value = varintField(firstField(fields, number));
    return value === undefined ? undefined : toNumber(value);
  };
  const readFloat = (number: number) => {
    const item = firstField(fields, number);
    return item && item.wire === WIRE_FIXED32 && typeof item.value === "number" ? item.value : undefined;
  };
  const state: Record<string, unknown> = {};
  const playState = readInt(1);
  if (playState !== undefined) state.play_state = hspPlayStateName(playState);
  const points = readInt(2);
  if (points !== undefined) state.points = points;
  const maxPoints = readInt(3);
  if (maxPoints !== undefined) state.max_points = maxPoints;
  const currentPoint = varintField(firstField(fields, 4));
  if (currentPoint !== undefined) state.current_point = toSigned32(currentPoint);
  const currentTime = varintField(firstField(fields, 5));
  if (currentTime !== undefined) state.current_time_ms = toSigned32(currentTime);
  const loop = readInt(6);
  if (loop !== undefined) state.loop = Boolean(loop);
  const playbackRate = readFloat(7);
  if (playbackRate !== undefined) state.playback_rate = Number(playbackRate.toFixed(4));
  const streamID = readInt(10);
  if (streamID !== undefined) state.stream_id = streamID;
  const tailPoint = varintField(firstField(fields, 11));
  if (tailPoint !== undefined) state.tail_point_stream_index = toSigned32(tailPoint);
  const threshold = readInt(12);
  if (threshold !== undefined) state.tail_point_stream_index_threshold = threshold;
  const pauseOnStarving = readInt(13);
  if (pauseOnStarving !== undefined) state.pause_on_starving = Boolean(pauseOnStarving);
  return state;
}

function parseError(bytes: Uint8Array): Record<string, unknown> {
  const fields = parseFields(bytes);
  const code = varintField(firstField(fields, 1));
  return {
    code: code === undefined ? 0 : toNumber(code),
    message: stringFromField(firstField(fields, 2)),
    data: stringFromField(firstField(fields, 3)),
  };
}

function parseHSPResponse(field: number, bytes: Uint8Array): Record<string, unknown> {
  const fields = parseFields(bytes);
  const result: Record<string, unknown> = { result_field: field };
  const stateBytes = bytesField(firstField(fields, 1));
  if (stateBytes) result.hsp_state = parseHSPState(stateBytes);
  return result;
}

function parseClockOffsetGet(bytes: Uint8Array): Record<string, unknown> {
  const fields = parseFields(bytes);
  const time = varintField(firstField(fields, 1));
  const clockOffset = varintField(firstField(fields, 2));
  const rtd = varintField(firstField(fields, 3));
  return {
    result_field: 712,
    clock_offset_get: {
      time: time === undefined ? 0 : toNumber(time),
      clock_offset: clockOffset === undefined ? 0 : zigZagToNumber(clockOffset),
      rtd: rtd === undefined ? 0 : toNumber(rtd),
    },
  };
}

function parseResponse(bytes: Uint8Array): Record<string, unknown> {
  const fields = parseFields(bytes);
  const idField = varintField(firstField(fields, 1));
  const response: Record<string, unknown> = { id: idField === undefined ? 0 : toNumber(idField), ok: true };
  const errorBytes = bytesField(firstField(fields, 2));
  if (errorBytes) {
    response.error = parseError(errorBytes);
    response.ok = false;
  }
  for (const field of fields) {
    if (field.field <= 10 || field.wire !== WIRE_LENGTH || !(field.value instanceof Uint8Array)) continue;
    if (HSP_RESPONSE_FIELDS.has(field.field)) Object.assign(response, parseHSPResponse(field.field, field.value));
    else if (field.field === 712) Object.assign(response, parseClockOffsetGet(field.value));
    else response.result_field = field.field;
  }
  return response;
}

function parseNotification(bytes: Uint8Array): Record<string, unknown> {
  const fields = parseFields(bytes);
  const idField = varintField(firstField(fields, 2));
  const notification: Record<string, unknown> = { id: idField === undefined ? 0 : toNumber(idField) };
  for (const field of fields) {
    if (field.wire !== WIRE_LENGTH || !HSP_NOTIFICATION_FIELDS.has(field.field) || !(field.value instanceof Uint8Array)) continue;
    const nested = parseFields(field.value);
    const stateBytes = bytesField(firstField(nested, 1));
    notification.event_field = field.field;
    if (stateBytes) notification.hsp_state = parseHSPState(stateBytes);
  }
  return notification;
}

export function decodeHandyRPCMessage(buffer: Uint8Array | ArrayBuffer): Record<string, unknown> {
  const bytes = buffer instanceof Uint8Array ? buffer : new Uint8Array(buffer);
  const fields = parseFields(bytes);
  const typeField = varintField(firstField(fields, 1));
  const type = typeField === undefined ? 0 : toNumber(typeField);
  if (type === MESSAGE_TYPE_RESPONSE) {
    const responseBytes = bytesField(firstField(fields, 4));
    return { type: "response", response: responseBytes ? parseResponse(responseBytes) : { id: 0, ok: false } };
  }
  if (type === MESSAGE_TYPE_NOTIFICATION) {
    const notificationBytes = bytesField(firstField(fields, 5));
    return { type: "notification", notification: notificationBytes ? parseNotification(notificationBytes) : {} };
  }
  return { type: "unknown", raw_type: type };
}
