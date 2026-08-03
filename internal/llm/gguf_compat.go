package llm

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	ggufVersion2 = 2
	ggufVersion3 = 3

	ggufTypeUint8   = 0
	ggufTypeInt8    = 1
	ggufTypeUint16  = 2
	ggufTypeInt16   = 3
	ggufTypeUint32  = 4
	ggufTypeInt32   = 5
	ggufTypeFloat32 = 6
	ggufTypeBool    = 7
	ggufTypeString  = 8
	ggufTypeArray   = 9
	ggufTypeUint64  = 10
	ggufTypeInt64   = 11
	ggufTypeFloat64 = 12

	maxGGUFMetadataEntries = 1 << 16
	maxGGUFArrayElements   = 1 << 20
	maxGGUFMetadataBytes   = 64 << 20
	maxGGUFKeyBytes        = 64 << 10
)

var (
	errNotGGUF                   = errors.New("file is not a GGUF model")
	errUnsupportedGGUFComponents = errors.New(
		"GGUF contains embedded audio, vision, or projector components that the managed llama.cpp text runner cannot load; use the model through Ollama or choose a text-only GGUF",
	)
	errUnsupportedGGUFSplit = errors.New(
		"split GGUF requires multiple shard files that the managed llama.cpp model store does not support",
	)
)

// inspectManagedGGUF reads only the bounded metadata section needed to reject
// files that require a runner contract beyond one text-only GGUF.
func inspectManagedGGUF(ctx context.Context, source io.Reader, size int64) error {
	if size <= 0 {
		return errNotGGUF
	}
	budget := int64(maxGGUFMetadataBytes)
	if size < budget {
		budget = size
	}
	reader := ggufMetadataReader{
		reader:    bufio.NewReaderSize(source, 64<<10),
		remaining: uint64(size),
		budget:    uint64(budget),
		ctx:       ctx,
	}
	metadataCount, err := readGGUFHeader(&reader)
	if err != nil {
		return err
	}

	for index := uint64(0); index < metadataCount; index++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		key, readErr := reader.readString(maxGGUFKeyBytes)
		if readErr != nil {
			return fmt.Errorf("GGUF metadata key %d is invalid: %w", index, readErr)
		}
		valueType, readErr := reader.readUint32()
		if readErr != nil {
			return fmt.Errorf("GGUF metadata value %q is invalid: %w", key, readErr)
		}
		if embeddedGGUFComponentKey(key) {
			return errUnsupportedGGUFComponents
		}
		if key == "split.count" {
			count, valueErr := reader.readUnsigned(valueType)
			if valueErr != nil {
				return fmt.Errorf("GGUF metadata value %q is invalid: %w", key, valueErr)
			}
			if count > 1 {
				return errUnsupportedGGUFSplit
			}
			continue
		}
		if readErr := reader.skipValue(valueType); readErr != nil {
			return fmt.Errorf("GGUF metadata value %q is invalid: %w", key, readErr)
		}
	}
	return nil
}

func readGGUFHeader(reader *ggufMetadataReader) (uint64, error) {
	magic, err := reader.readBytes(4)
	if err != nil || string(magic) != "GGUF" {
		return 0, errNotGGUF
	}
	version, err := reader.readUint32()
	if err != nil || (version != ggufVersion2 && version != ggufVersion3) {
		return 0, errors.New("GGUF metadata has an unsupported version")
	}
	tensorCount, err := reader.readUint64()
	if err != nil || tensorCount == 0 {
		return 0, errors.New("GGUF metadata has no tensors")
	}
	metadataCount, err := reader.readUint64()
	if err != nil || metadataCount == 0 || metadataCount > maxGGUFMetadataEntries {
		return 0, errors.New("GGUF metadata entry count is invalid")
	}
	return metadataCount, nil
}

func embeddedGGUFComponentKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, component := range []string{"audio", "vision", "projector"} {
		if strings.HasPrefix(key, component+".") || strings.Contains(key, "."+component+".") {
			return true
		}
	}
	return strings.HasPrefix(key, "clip.")
}

type ggufMetadataReader struct {
	reader    *bufio.Reader
	remaining uint64
	budget    uint64
	ctx       context.Context
}

func (r *ggufMetadataReader) readBytes(count uint64) ([]byte, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	if count > r.remaining {
		return nil, io.ErrUnexpectedEOF
	}
	if count > r.budget {
		return nil, errors.New("metadata exceeds the scan limit")
	}
	if count > uint64(int(^uint(0)>>1)) {
		return nil, errors.New("metadata value is too large")
	}
	payload := make([]byte, int(count))
	if _, err := io.ReadFull(r.reader, payload); err != nil {
		return nil, err
	}
	r.remaining -= count
	r.budget -= count
	return payload, nil
}

func (r *ggufMetadataReader) skip(count uint64) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	if count > r.remaining {
		return io.ErrUnexpectedEOF
	}
	if count > r.budget {
		return errors.New("metadata exceeds the scan limit")
	}
	// #nosec G115 -- count is bounded above by the 64 MiB metadata budget.
	if _, err := io.CopyN(io.Discard, r.reader, int64(count)); err != nil {
		return err
	}
	r.remaining -= count
	r.budget -= count
	return nil
}

func (r *ggufMetadataReader) readUint32() (uint32, error) {
	payload, err := r.readBytes(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(payload), nil
}

func (r *ggufMetadataReader) readUint64() (uint64, error) {
	payload, err := r.readBytes(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(payload), nil
}

func (r *ggufMetadataReader) readString(limit uint64) (string, error) {
	length, err := r.readUint64()
	if err != nil {
		return "", err
	}
	if length > limit {
		return "", errors.New("string exceeds the scan limit")
	}
	payload, err := r.readBytes(length)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func (r *ggufMetadataReader) skipString() error {
	length, err := r.readUint64()
	if err != nil {
		return err
	}
	return r.skip(length)
}

func (r *ggufMetadataReader) readUnsigned(valueType uint32) (uint64, error) {
	switch valueType {
	case ggufTypeUint8:
		payload, err := r.readBytes(1)
		if err != nil {
			return 0, err
		}
		return uint64(payload[0]), nil
	case ggufTypeUint16:
		payload, err := r.readBytes(2)
		if err != nil {
			return 0, err
		}
		return uint64(binary.LittleEndian.Uint16(payload)), nil
	case ggufTypeUint32:
		value, err := r.readUint32()
		return uint64(value), err
	case ggufTypeUint64:
		return r.readUint64()
	default:
		return 0, fmt.Errorf("type %d is not an unsigned integer", valueType)
	}
}

func (r *ggufMetadataReader) skipValue(valueType uint32) error {
	if size, ok := fixedGGUFValueSize(valueType); ok {
		return r.skip(size)
	}
	if valueType == ggufTypeString {
		return r.skipString()
	}
	if valueType != ggufTypeArray {
		return fmt.Errorf("unknown GGUF value type %d", valueType)
	}
	elementType, err := r.readUint32()
	if err != nil {
		return err
	}
	count, err := r.readUint64()
	if err != nil {
		return err
	}
	if count > maxGGUFArrayElements {
		return errors.New("array exceeds the scan limit")
	}
	if size, ok := fixedGGUFValueSize(elementType); ok {
		if count != 0 && size > ^uint64(0)/count {
			return errors.New("array size overflows")
		}
		return r.skip(size * count)
	}
	if elementType != ggufTypeString {
		return fmt.Errorf("unsupported GGUF array type %d", elementType)
	}
	for index := uint64(0); index < count; index++ {
		if err := r.skipString(); err != nil {
			return err
		}
	}
	return nil
}

func fixedGGUFValueSize(valueType uint32) (uint64, bool) {
	switch valueType {
	case ggufTypeUint8, ggufTypeInt8, ggufTypeBool:
		return 1, true
	case ggufTypeUint16, ggufTypeInt16:
		return 2, true
	case ggufTypeUint32, ggufTypeInt32, ggufTypeFloat32:
		return 4, true
	case ggufTypeUint64, ggufTypeInt64, ggufTypeFloat64:
		return 8, true
	default:
		return 0, false
	}
}
