// Handy BLE protobuf codec (RPC request encoding + response/notification decoding).
// Extracted from app.js to keep modules within the size norms (goals-and-guardrails).

const bluetoothDecoder = new TextDecoder();
const WIRE_VARINT = 0;
const WIRE_LENGTH = 2;
const WIRE_FIXED32 = 5;
const MESSAGE_TYPE_REQUEST = 1;
const MESSAGE_TYPE_RESPONSE = 3;
const MESSAGE_TYPE_NOTIFICATION = 4;
const HSP_POINT_PROTOCOL_MAX = 1000;
const REQUEST_FIELDS = {
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

function concatBytes(parts) {
  const total = parts.reduce((sum, part) => sum + part.length, 0);
  const output = new Uint8Array(total);
  let offset = 0;
  parts.forEach((part) => {
    output.set(part, offset);
    offset += part.length;
  });
  return output;
}

function encodeVarint(value) {
  let next = typeof value === "bigint" ? value : BigInt(Math.max(0, Number(value) || 0));
  const bytes = [];
  while (next > 0x7fn) {
    bytes.push(Number((next & 0x7fn) | 0x80n));
    next >>= 7n;
  }
  bytes.push(Number(next));
  return Uint8Array.from(bytes);
}

function encodeSignedVarint(value) {
  let next = BigInt(Math.trunc(Number(value) || 0));
  if (next < 0) {
    next = BigInt.asUintN(64, next);
  }
  return encodeVarint(next);
}

function encodeZigZag64(value) {
  const n = BigInt(Math.trunc(Number(value) || 0));
  return encodeVarint((n << 1n) ^ (n >> 63n));
}

function fieldKey(field, wireType) {
  return encodeVarint((BigInt(field) << 3n) | BigInt(wireType));
}

function uintField(field, value) {
  return concatBytes([fieldKey(field, WIRE_VARINT), encodeVarint(value)]);
}

function intField(field, value) {
  return concatBytes([fieldKey(field, WIRE_VARINT), encodeSignedVarint(value)]);
}

function sint64Field(field, value) {
  return concatBytes([fieldKey(field, WIRE_VARINT), encodeZigZag64(value)]);
}

function boolField(field, value) {
  return uintField(field, value ? 1 : 0);
}

function floatField(field, value) {
  const bytes = new Uint8Array(4);
  new DataView(bytes.buffer).setFloat32(0, Number(value) || 0, true);
  return concatBytes([fieldKey(field, WIRE_FIXED32), bytes]);
}

function lengthField(field, bytes) {
  return concatBytes([fieldKey(field, WIRE_LENGTH), encodeVarint(bytes.length), bytes]);
}

function strokePercentValue(value, fallback) {
  const number = Number(value);
  const safe = Number.isFinite(number) ? number : fallback;
  const percent = safe >= 0 && safe <= 1 ? safe * 100 : safe;
  return Math.max(0, Math.min(100, percent));
}

function pointMessage(point = {}) {
  const normalized = Math.max(0, Math.min(100, Number(point.x ?? point.pos ?? 50) || 0));
  return concatBytes([
    uintField(1, Math.max(0, Math.round(Number(point.t ?? 0) || 0))),
    uintField(2, Math.round((normalized / 100) * HSP_POINT_PROTOCOL_MAX)),
  ]);
}

function requestBodyForPath(path, body = {}) {
  if (path === "slider/stroke") {
    return concatBytes([
      floatField(1, strokePercentValue(body.min, 0)),
      floatField(2, strokePercentValue(body.max, 100)),
    ]);
  }
  if (path === "hsp/setup") {
    return uintField(1, body.stream_id ?? 0);
  }
  if (path === "hsp/add") {
    return concatBytes([
      ...(Array.isArray(body.points) ? body.points : []).map((point) => lengthField(1, pointMessage(point))),
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
  if (path === "hsp/stop" || path === "hsp/state" || path === "clock/offset/get") {
    return new Uint8Array();
  }
  throw new Error(`Bluetooth command is not implemented: ${path}`);
}

export function encodeHandyRequest(path, body = {}, id = 1) {
  const field = REQUEST_FIELDS[path];
  if (!field) {
    throw new Error(`Bluetooth command is not implemented: ${path}`);
  }
  const payload = requestBodyForPath(path, body);
  const request = concatBytes([
    lengthField(field, payload),
    uintField(2, id),
  ]);
  return concatBytes([
    uintField(1, MESSAGE_TYPE_REQUEST),
    lengthField(2, request),
  ]);
}

function readVarint(bytes, offset) {
  let shift = 0n;
  let value = 0n;
  let index = offset;
  while (index < bytes.length) {
    const byte = BigInt(bytes[index]);
    value |= (byte & 0x7fn) << shift;
    index += 1;
    if ((byte & 0x80n) === 0n) {
      return { value, offset: index };
    }
    shift += 7n;
  }
  throw new Error("Truncated varint");
}

function toNumber(value) {
  const maxSafe = BigInt(Number.MAX_SAFE_INTEGER);
  return Number(value > maxSafe ? maxSafe : value);
}

function toSigned32(value) {
  const unsigned = Number(value & 0xffffffffn);
  return unsigned >= 0x80000000 ? unsigned - 0x100000000 : unsigned;
}

function zigZagToNumber(value) {
  const decoded = (value >> 1n) ^ (-(value & 1n));
  return Number(decoded);
}

function parseFields(bytes) {
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const fields = [];
  let offset = 0;
  while (offset < bytes.length) {
    const key = readVarint(bytes, offset);
    offset = key.offset;
    const field = Number(key.value >> 3n);
    const wire = Number(key.value & 0x7n);
    let value;
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

function firstField(fields, number) {
  return fields.find((item) => item.field === number);
}

function stringFromField(item) {
  if (!item || item.wire !== WIRE_LENGTH) {
    return "";
  }
  return bluetoothDecoder.decode(item.value);
}

function hspPlayStateName(value) {
  return {
    0: "not_initialized",
    1: "playing",
    2: "stopped",
    3: "paused",
    4: "starving",
  }[Number(value)] || String(value);
}

function parseHSPState(bytes) {
  const fields = parseFields(bytes);
  const readInt = (number) => {
    const item = firstField(fields, number);
    return item && item.wire === WIRE_VARINT ? toNumber(item.value) : undefined;
  };
  const readFloat = (number) => {
    const item = firstField(fields, number);
    return item && item.wire === WIRE_FIXED32 ? Number(item.value) : undefined;
  };
  const state = {};
  const playState = readInt(1);
  if (playState !== undefined) state.play_state = hspPlayStateName(playState);
  const points = readInt(2);
  if (points !== undefined) state.points = points;
  const maxPoints = readInt(3);
  if (maxPoints !== undefined) state.max_points = maxPoints;
  const currentPoint = firstField(fields, 4);
  if (currentPoint && currentPoint.wire === WIRE_VARINT) state.current_point = toSigned32(currentPoint.value);
  const currentTime = firstField(fields, 5);
  if (currentTime && currentTime.wire === WIRE_VARINT) state.current_time_ms = toSigned32(currentTime.value);
  const loop = readInt(6);
  if (loop !== undefined) state.loop = Boolean(loop);
  const playbackRate = readFloat(7);
  if (playbackRate !== undefined) state.playback_rate = Number(playbackRate.toFixed(4));
  const streamID = readInt(10);
  if (streamID !== undefined) state.stream_id = streamID;
  const tailPoint = firstField(fields, 11);
  if (tailPoint && tailPoint.wire === WIRE_VARINT) state.tail_point_stream_index = toSigned32(tailPoint.value);
  const threshold = readInt(12);
  if (threshold !== undefined) state.tail_point_stream_index_threshold = threshold;
  const pauseOnStarving = readInt(13);
  if (pauseOnStarving !== undefined) state.pause_on_starving = Boolean(pauseOnStarving);
  return state;
}

function parseError(bytes) {
  const fields = parseFields(bytes);
  const code = firstField(fields, 1);
  return {
    code: code && code.wire === WIRE_VARINT ? toNumber(code.value) : 0,
    message: stringFromField(firstField(fields, 2)),
    data: stringFromField(firstField(fields, 3)),
  };
}

function parseHSPResponse(field, bytes) {
  const fields = parseFields(bytes);
  const stateField = firstField(fields, 1);
  const result = { result_field: field };
  if (stateField && stateField.wire === WIRE_LENGTH) {
    result.hsp_state = parseHSPState(stateField.value);
  }
  return result;
}

function parseClockOffsetGet(bytes) {
  const fields = parseFields(bytes);
  const time = firstField(fields, 1);
  const clockOffset = firstField(fields, 2);
  const rtd = firstField(fields, 3);
  return {
    result_field: 712,
    clock_offset_get: {
      time: time && time.wire === WIRE_VARINT ? toNumber(time.value) : 0,
      clock_offset: clockOffset && clockOffset.wire === WIRE_VARINT ? zigZagToNumber(clockOffset.value) : 0,
      rtd: rtd && rtd.wire === WIRE_VARINT ? toNumber(rtd.value) : 0,
    },
  };
}

function parseResponse(bytes) {
  const fields = parseFields(bytes);
  const idField = firstField(fields, 1);
  const response = {
    id: idField && idField.wire === WIRE_VARINT ? toNumber(idField.value) : 0,
    ok: true,
  };
  const errorField = firstField(fields, 2);
  if (errorField && errorField.wire === WIRE_LENGTH) {
    response.error = parseError(errorField.value);
    response.ok = false;
  }
  fields.forEach((field) => {
    if (field.field <= 10 || field.wire !== WIRE_LENGTH) {
      return;
    }
    if (HSP_RESPONSE_FIELDS.has(field.field)) {
      Object.assign(response, parseHSPResponse(field.field, field.value));
    } else if (field.field === 712) {
      Object.assign(response, parseClockOffsetGet(field.value));
    } else {
      response.result_field = field.field;
    }
  });
  return response;
}

function parseNotification(bytes) {
  const fields = parseFields(bytes);
  const idField = firstField(fields, 2);
  const notification = {
    id: idField && idField.wire === WIRE_VARINT ? toNumber(idField.value) : 0,
  };
  fields.forEach((field) => {
    if (field.wire !== WIRE_LENGTH || !HSP_NOTIFICATION_FIELDS.has(field.field)) {
      return;
    }
    const nested = parseFields(field.value);
    const stateField = firstField(nested, 1);
    notification.event_field = field.field;
    if (stateField && stateField.wire === WIRE_LENGTH) {
      notification.hsp_state = parseHSPState(stateField.value);
    }
  });
  return notification;
}

export function decodeHandyRPCMessage(buffer) {
  const bytes = buffer instanceof Uint8Array ? buffer : new Uint8Array(buffer);
  const fields = parseFields(bytes);
  const typeField = firstField(fields, 1);
  const type = typeField && typeField.wire === WIRE_VARINT ? toNumber(typeField.value) : 0;
  if (type === MESSAGE_TYPE_RESPONSE) {
    const responseField = firstField(fields, 4);
    return {
      type: "response",
      response: responseField && responseField.wire === WIRE_LENGTH ? parseResponse(responseField.value) : { id: 0, ok: false },
    };
  }
  if (type === MESSAGE_TYPE_NOTIFICATION) {
    const notificationField = firstField(fields, 5);
    return {
      type: "notification",
      notification: notificationField && notificationField.wire === WIRE_LENGTH ? parseNotification(notificationField.value) : {},
    };
  }
  return { type: "unknown", raw_type: type };
}
