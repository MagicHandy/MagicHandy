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

describe("Handy BLE codec", () => {
  it("encodes the backend's semantic stroke percentages without guessing normalized units", () => {
    expect(strokeWindow(encodeHandyRequest("slider/stroke", { min: 1, max: 40 }, 7))).toEqual({
      min: 1,
      max: 40,
    });
  });

  it("clamps stroke percentages at the protocol boundary", () => {
    expect(strokeWindow(encodeHandyRequest("slider/stroke", { min: -20, max: 120 }))).toEqual({
      min: 0,
      max: 100,
    });
  });

  it("rejects truncated length-delimited fields", () => {
    expect(() => decodeHandyRPCMessage(Uint8Array.from([0x12, 0x05, 0x01]))).toThrow(/truncated length-delimited/i);
  });

  it("rejects protobuf varints wider than uint64", () => {
    expect(() => decodeHandyRPCMessage(Uint8Array.from([
      0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x00,
    ]))).toThrow(/exceeds 64 bits/i);
  });
});
