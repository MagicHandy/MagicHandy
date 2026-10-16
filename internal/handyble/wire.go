package handyble

import (
	"encoding/binary"
	"errors"
	"math"
)

// MaxFrameBytes is the firmware RPC frame ceiling, before negotiated MTU limits.
const MaxFrameBytes = 512

func appendUint(dst []byte, field uint32, value uint64) []byte {
	dst = binary.AppendUvarint(dst, uint64(field)<<3)
	return binary.AppendUvarint(dst, value)
}

func appendBytes(dst []byte, field uint32, data []byte) []byte {
	dst = binary.AppendUvarint(dst, uint64(field)<<3|2)
	dst = binary.AppendUvarint(dst, uint64(len(data)))
	return append(dst, data...)
}

func appendFloat(dst []byte, field uint32, value float32) []byte {
	dst = binary.AppendUvarint(dst, uint64(field)<<3|5)
	return binary.LittleEndian.AppendUint32(dst, math.Float32bits(value))
}

func boolUint(v bool) uint64 {
	if v {
		return 1
	}
	return 0
}

type wireField struct {
	number uint32
	wire   byte
	value  uint64
	data   []byte
}
type wireFields []wireField

func (f wireFields) uint(n uint32) uint64 {
	for i := len(f) - 1; i >= 0; i-- {
		if f[i].number == n && f[i].wire == 0 {
			return f[i].value
		}
	}
	return 0
}
func (f wireFields) bytes(n uint32) []byte {
	for i := len(f) - 1; i >= 0; i-- {
		if f[i].number == n && f[i].wire == 2 {
			return f[i].data
		}
	}
	return nil
}

func parseFields(data []byte) (wireFields, error) {
	if len(data) > MaxFrameBytes {
		return nil, errors.New("oversized Handy RPC frame")
	}
	var fields wireFields
	for len(data) > 0 {
		key, n := binary.Uvarint(data)
		if n <= 0 || key>>3 == 0 || key>>3 > (1<<29)-1 {
			return nil, errors.New("invalid protobuf field")
		}
		data = data[n:]
		f := wireField{number: uint32(key >> 3), wire: byte(key & 7)}
		switch f.wire {
		case 0:
			v, n := binary.Uvarint(data)
			if n <= 0 {
				return nil, errors.New("invalid protobuf varint")
			}
			f.value, data = v, data[n:]
		case 2:
			length, n := binary.Uvarint(data)
			if n <= 0 || length > MaxFrameBytes {
				return nil, errors.New("truncated protobuf field")
			}
			data = data[n:]
			if int(length) > len(data) {
				return nil, errors.New("truncated protobuf field")
			}
			f.data, data = data[:int(length)], data[int(length):]
		case 1, 5:
			n := 4
			if f.wire == 1 {
				n = 8
			}
			if len(data) < n {
				return nil, errors.New("truncated fixed protobuf field")
			}
			f.data, data = data[:n], data[n:]
		default:
			return nil, errors.New("unsupported protobuf wire type")
		}
		fields = append(fields, f)
	}
	return fields, nil
}
