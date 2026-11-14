// Package winresource turns the brand icon into the Windows resource object
// that gives magichandy.exe its icon.
//
// The Go linker links every *_windows_amd64.syso file it finds in a package
// directory. A resource object is a COFF file with one .rsrc section holding
// the Windows resource tree, so no extra tool or dependency is needed at build
// time: the committed object is rebuilt from the committed icon by
// cmd/windows-resource, and a test fails when the two drift apart.
package winresource

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

// Windows resource type identifiers and the fixed IDs this package writes.
const (
	typeIcon      = 3  // RT_ICON: one image of an icon
	typeGroupIcon = 14 // RT_GROUP_ICON: the directory that ties the images together
	groupID       = 1
	languageEnUS  = 0x0409

	machineAMD64        = 0x8664
	relocationADDR32NB  = 0x0003 // image-relative 32-bit address
	sectionInitialized  = 0x00000040
	sectionReadable     = 0x40000000
	symbolClassStatic   = 3
	subdirectoryFlag    = 0x80000000
	iconDirectoryType   = 1
	iconEntrySize       = 16
	groupEntrySize      = 14
	maxIconImages       = 64
	maxIconBytes        = 16 << 20 // far above any real icon; keeps every size within 32 bits
	resourceDataPadding = 8
)

// Image is one picture inside an icon file.
type Image struct {
	Width, Height uint8 // 0 means 256
	ColorCount    uint8
	Planes        uint16
	BitCount      uint16
	Data          []byte // a DIB without its file header, or a PNG
}

// ParseICO reads the images of a Windows .ico file.
func ParseICO(ico []byte) ([]Image, error) {
	if len(ico) < 6 {
		return nil, errors.New("icon file is too short")
	}
	if len(ico) > maxIconBytes {
		return nil, fmt.Errorf("icon file is %d bytes, over the %d byte limit", len(ico), maxIconBytes)
	}
	if binary.LittleEndian.Uint16(ico[0:]) != 0 || binary.LittleEndian.Uint16(ico[2:]) != iconDirectoryType {
		return nil, errors.New("not a Windows icon file")
	}
	count := int(binary.LittleEndian.Uint16(ico[4:]))
	if count == 0 || count > maxIconImages {
		return nil, fmt.Errorf("icon file holds %d images, want 1 to %d", count, maxIconImages)
	}
	if len(ico) < 6+count*iconEntrySize {
		return nil, errors.New("icon directory is truncated")
	}
	images := make([]Image, 0, count)
	for i := range count {
		entry := ico[6+i*iconEntrySize:]
		size := binary.LittleEndian.Uint32(entry[8:])
		offset := binary.LittleEndian.Uint32(entry[12:])
		if size == 0 || uint64(offset)+uint64(size) > uint64(len(ico)) {
			return nil, fmt.Errorf("icon image %d lies outside the file", i)
		}
		images = append(images, Image{
			Width:      entry[0],
			Height:     entry[1],
			ColorCount: entry[2],
			Planes:     binary.LittleEndian.Uint16(entry[4:]),
			BitCount:   binary.LittleEndian.Uint16(entry[6:]),
			Data:       ico[offset : offset+size],
		})
	}
	return images, nil
}

// Object builds the COFF resource object for an amd64 Windows executable whose
// icon is the given .ico file. The output depends only on the input bytes.
func Object(ico []byte) ([]byte, error) {
	images, err := ParseICO(ico)
	if err != nil {
		return nil, err
	}
	section, relocations := resourceSection(images)

	const fileHeaderSize, sectionHeaderSize, relocationSize, symbolSize = 20, 40, 10, 18
	dataOffset := uint32(fileHeaderSize + sectionHeaderSize)
	relocationOffset := dataOffset + u32(len(section))
	symbolOffset := relocationOffset + u32(len(relocations)*relocationSize)

	var out bytes.Buffer
	put := func(values ...any) {
		for _, value := range values {
			// bytes.Buffer never fails to write.
			_ = binary.Write(&out, binary.LittleEndian, value)
		}
	}
	// File header: one section, one symbol, no optional header.
	put(uint16(machineAMD64), uint16(1), uint32(0), symbolOffset, uint32(1), uint16(0), uint16(0))
	// Section header.
	out.Write([]byte{'.', 'r', 's', 'r', 'c', 0, 0, 0})
	put(uint32(0), uint32(0), u32(len(section)), dataOffset, relocationOffset, uint32(0),
		u16(len(relocations)), uint16(0), uint32(sectionInitialized|sectionReadable))
	out.Write(section)
	// Every data entry holds an address the linker must make image-relative.
	for _, at := range relocations {
		put(at, uint32(0), uint16(relocationADDR32NB))
	}
	// The one symbol the relocations refer to: the start of the section.
	out.Write([]byte{'.', 'r', 's', 'r', 'c', 0, 0, 0})
	put(uint32(0), int16(1), uint16(0), uint8(symbolClassStatic), uint8(0))
	// An empty string table is its own four-byte length.
	put(uint32(4))
	return out.Bytes(), nil
}

// resourceSection lays out the three-level resource tree (type, ID, language)
// followed by the data entries and the data itself. It returns the section and
// the offsets of the address fields that need a relocation.
func resourceSection(images []Image) ([]byte, []uint32) {
	const directorySize, entrySize, dataEntrySize = 16, 8, 16
	count := len(images)

	// Offsets of each directory, in the order they are written.
	root := 0
	iconIDs := root + directorySize + 2*entrySize
	groupIDs := iconIDs + directorySize + count*entrySize
	iconLanguages := groupIDs + directorySize + entrySize
	groupLanguage := iconLanguages + count*(directorySize+entrySize)
	dataEntries := groupLanguage + directorySize + entrySize
	data := dataEntries + (count+1)*dataEntrySize

	group := groupDirectory(images)
	blobs := make([][]byte, 0, count+1)
	for _, image := range images {
		blobs = append(blobs, image.Data)
	}
	blobs = append(blobs, group)

	offsets := make([]int, len(blobs))
	size := data
	for i, blob := range blobs {
		offsets[i] = size
		size += align(len(blob))
	}

	section := make([]byte, size)
	le := binary.LittleEndian
	directory := func(at, ids int) {
		le.PutUint16(section[at+14:], u16(ids)) // number of ID entries; no named entries
	}
	entry := func(at int, id uint32, target int, subdirectory bool) {
		le.PutUint32(section[at:], id)
		value := u32(target)
		if subdirectory {
			value |= subdirectoryFlag
		}
		le.PutUint32(section[at+4:], value)
	}

	// Level 1: resource types, in ascending order.
	directory(root, 2)
	entry(root+directorySize, typeIcon, iconIDs, true)
	entry(root+directorySize+entrySize, typeGroupIcon, groupIDs, true)

	// Level 2: one ID per image, and the single group.
	directory(iconIDs, count)
	for i := range count {
		entry(iconIDs+directorySize+i*entrySize, u32(i+1), iconLanguages+i*(directorySize+entrySize), true)
	}
	directory(groupIDs, 1)
	entry(groupIDs+directorySize, groupID, groupLanguage, true)

	// Level 3: one language each, pointing at a data entry.
	for i := range count {
		at := iconLanguages + i*(directorySize+entrySize)
		directory(at, 1)
		entry(at+directorySize, languageEnUS, dataEntries+i*dataEntrySize, false)
	}
	directory(groupLanguage, 1)
	entry(groupLanguage+directorySize, languageEnUS, dataEntries+count*dataEntrySize, false)

	// Data entries and data.
	relocations := make([]uint32, 0, len(blobs))
	for i, blob := range blobs {
		at := dataEntries + i*dataEntrySize
		le.PutUint32(section[at:], u32(offsets[i])) // made image-relative by the linker
		le.PutUint32(section[at+4:], u32(len(blob)))
		relocations = append(relocations, u32(at))
		copy(section[offsets[i]:], blob)
	}
	return section, relocations
}

// groupDirectory is the icon file's directory with each file offset replaced by
// the resource ID of that image.
func groupDirectory(images []Image) []byte {
	out := make([]byte, 6+len(images)*groupEntrySize)
	le := binary.LittleEndian
	le.PutUint16(out[2:], iconDirectoryType)
	le.PutUint16(out[4:], u16(len(images)))
	for i, image := range images {
		entry := out[6+i*groupEntrySize:]
		entry[0], entry[1], entry[2] = image.Width, image.Height, image.ColorCount
		le.PutUint16(entry[4:], image.Planes)
		le.PutUint16(entry[6:], image.BitCount)
		le.PutUint32(entry[8:], u32(len(image.Data)))
		le.PutUint16(entry[12:], u16(i+1))
	}
	return out
}

// u32 and u16 narrow sizes and counts that ParseICO has already bounded: an icon
// is at most maxIconBytes long and holds at most maxIconImages images.
func u32(n int) uint32 {
	return uint32(n) // #nosec G115 -- bounded by maxIconBytes
}

func u16(n int) uint16 {
	return uint16(n) // #nosec G115 -- bounded by maxIconImages
}

func align(n int) int {
	return (n + resourceDataPadding - 1) / resourceDataPadding * resourceDataPadding
}
