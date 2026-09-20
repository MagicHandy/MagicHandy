import { describe, expect, it } from "vitest";
import { decodeHandyRPCMessage, encodeHandyRequest } from "./handy-ble-codec";

interface ParsedField {
  field: number;
  wire: number;
  value: bigint | Uint8Array | number;
}

function readVarint(bytes: Uint8Array, start: number) {
  let value = 0n;
  let shift = 0n;
  let offset = start;
  while (offset < bytes.length) {
    const byte = BigInt(bytes[offset]);
    value |= (byte & 0x7fn) << shift;
    offset += 1;
    if ((byte & 0x80n) === 0n) return { value, offset };
    shift += 7n;
  }
  throw new Error("truncated test varint");
}

function parseFields(bytes: Uint8Array): ParsedField[] {
  const fields: ParsedField[] = [];
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  let offset = 0;
  while (offset < bytes.length) {
    const key = readVarint(bytes, offset);
    offset = key.offset;
    const field = Number(key.value >> 3n);
    const wire = Number(key.value & 7n);
    if (wire === 0) {
      const value = readVarint(bytes, offset);
      fields.push({ field, wire, value: value.value });
      offset = value.offset;
      continue;
    }
    if (wire === 2) {
      const length = readVarint(bytes, offset);
      offset = length.offset;
      const size = Number(length.value);
      fields.push({ field, wire, value: bytes.slice(offset, offset + size) });
      offset += size;
      continue;
    }
    if (wire === 5) {
      fields.push({ field, wire, value: view.getFloat32(offset, true) });
      offset += 4;
      continue;
    }
    throw new Error(`unsupported test wire type ${wire}`);
  }
  return fields;
}

function bytesValue(fields: ParsedField[], field: number): Uint8Array {
  const value = fields.find((candidate) => candidate.field === field)?.value;
  if (!(value instanceof Uint8Array)) throw new Error(`field ${field} is not bytes`);
  return value;
}

function strokeWindow(encoded: Uint8Array) {
  const request = bytesValue(parseFields(encoded), 2);
  const body = bytesValue(parseFields(request), 841);
  const fields = parseFields(body);
  return {
    min: Number(fields.find((field) => field.field === 1)?.value),
    max: Number(fields.find((field) => field.field === 2)?.value),
  };
}

function hspPositions(encoded: Uint8Array): number[] {
  const request = bytesValue(parseFields(encoded), 2);
  const body = bytesValue(parseFields(request), 861);
  return parseFields(body)
    .filter((field) => field.field === 1 && field.value instanceof Uint8Array)
    .map((field) => Number(parseFields(field.value as Uint8Array).find((pointField) => pointField.field === 2)?.value));
}

describe("Handy BLE codec", () => {
  it("converts backend stroke percentages to normalized wire units exactly once", () => {
    const stroke = strokeWindow(encodeHandyRequest("slider/stroke", { min: 1, max: 40 }, 7));
    expect(stroke.min).toBeCloseTo(0.01);
    expect(stroke.max).toBeCloseTo(0.4);
  });

  it("clamps stroke percentages at the protocol boundary", () => {
    expect(strokeWindow(encodeHandyRequest("slider/stroke", { min: -20, max: 120 }))).toEqual({
      min: 0,
      max: 1,
    });
  });

  it("encodes HSP points as integer percent without saturating firmware above 100", () => {
    const encoded = encodeHandyRequest("hsp/add", {
      points: [{ t: 0, x: 25.25 }, { t: 125, x: 75.75 }],
    });
    expect(hspPositions(encoded)).toEqual([25, 76]);
  });

  it("rejects truncated length-delimited fields", () => {
    expect(() => decodeHandyRPCMessage(Uint8Array.from([0x12, 0x05, 0x01]))).toThrow(/truncated length-delimited/i);
  });

  it("decodes proto3 omitted zero values for an uninitialized stream zero", () => {
    // RpcMessage.notification -> NotificationHspStateChanged (861) -> HspState.
    // Field numbers and defaults from the manufacturer's public RPC definitions.
    const result = decodeHandyRPCMessage(Uint8Array.from([0x08, 0x04, 0x2a, 0x05, 0xea, 0x35, 0x02, 0x0a, 0x00]));
    expect(result).toMatchObject({ type: "notification", notification: { hsp_state: { play_state: "not_initialized", stream_id: 0, points: 0, current_point: 0, current_time_ms: 0 } } });
  });

  it("omits server time without a clock and encodes the backend's absolute tail index", () => {
    const play = bytesValue(parseFields(bytesValue(parseFields(encodeHandyRequest("hsp/play", { start_time: 0 })), 2)), 863);
    expect(parseFields(play).some((field) => field.field === 2)).toBe(false);
    const add = bytesValue(parseFields(bytesValue(parseFields(encodeHandyRequest("hsp/add", { points: [{ t: 100, x: 50 }], tail_point_stream_index: 99 })), 2)), 861);
    expect(parseFields(add).find((field) => field.field === 3)?.value).toBe(99n);
  });

  it("ignores unknown fixed64 fields without discarding supported notifications", () => {
    const result = decodeHandyRPCMessage(Uint8Array.from([0x08, 0x04, 0x2a, 0x05, 0xea, 0x35, 0x02, 0x0a, 0x00, 0x31, 0, 0, 0, 0, 0, 0, 0, 0]));
    expect(result).toMatchObject({ type: "notification" });
  });

  it("rejects protobuf varints wider than uint64", () => {
    expect(() => decodeHandyRPCMessage(Uint8Array.from([
      0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x00,
    ]))).toThrow(/exceeds 64 bits/i);
  });
});
