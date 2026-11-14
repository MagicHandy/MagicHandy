package winresource

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

const (
	brandIcon       = "../../assets/brand/magichandy.ico"
	committedObject = "../../cmd/magichandy/resource_windows_amd64.syso"
)

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.FromSlash(path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func TestBrandIconCarriesTheSizesWindowsAsksFor(t *testing.T) {
	images, err := ParseICO(readFile(t, brandIcon))
	if err != nil {
		t.Fatalf("ParseICO: %v", err)
	}
	have := map[int]bool{}
	for _, image := range images {
		size := int(image.Width)
		if size == 0 {
			size = 256
		}
		if image.Width != image.Height {
			t.Errorf("image %d x %d is not square", image.Width, image.Height)
		}
		if image.BitCount != 32 {
			t.Errorf("%d px image is %d-bit, want 32-bit with alpha", size, image.BitCount)
		}
		have[size] = true
	}
	// Taskbar, title bar, Alt+Tab, Start and Explorer at 100% to 200% scaling.
	for _, size := range []int{16, 20, 24, 32, 40, 48, 64, 256} {
		if !have[size] {
			t.Errorf("brand icon has no %d px image", size)
		}
	}
}

// The executable's icon is whatever is committed next to main.go, so the
// committed object must be exactly what the committed icon produces.
func TestCommittedResourceObjectMatchesTheBrandIcon(t *testing.T) {
	want, err := Object(readFile(t, brandIcon))
	if err != nil {
		t.Fatalf("Object: %v", err)
	}
	if got := readFile(t, committedObject); !bytes.Equal(got, want) {
		t.Fatalf("%s is stale (%d bytes, want %d): run `go run ./cmd/windows-resource`", committedObject, len(got), len(want))
	}
}

// linkable parses the object for the brand icon and returns what the tests inspect.
func linkable(t *testing.T) (images []Image, section *pe.Section, data []byte, symbols []*pe.Symbol) {
	t.Helper()
	ico := readFile(t, brandIcon)
	images, err := ParseICO(ico)
	if err != nil {
		t.Fatalf("ParseICO: %v", err)
	}
	object, err := Object(ico)
	if err != nil {
		t.Fatalf("Object: %v", err)
	}
	file, err := pe.NewFile(bytes.NewReader(object))
	if err != nil {
		t.Fatalf("object is not a readable COFF file: %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })
	if file.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
		t.Fatalf("machine = %#x, want amd64", file.Machine)
	}
	if len(file.Sections) != 1 || file.Sections[0].Name != ".rsrc" {
		t.Fatalf("sections = %v, want one .rsrc section", file.Sections)
	}
	section = file.Sections[0]
	data, err = section.Data()
	if err != nil {
		t.Fatalf("section data: %v", err)
	}
	return images, section, data, file.Symbols
}

func TestObjectIsALinkableResourceSection(t *testing.T) {
	images, section, data, symbols := linkable(t)

	// One relocation per data entry: every image plus the group directory.
	if len(section.Relocs) != len(images)+1 {
		t.Fatalf("relocations = %d, want %d", len(section.Relocs), len(images)+1)
	}
	if len(symbols) != 1 || symbols[0].Name != ".rsrc" || symbols[0].SectionNumber != 1 {
		t.Fatalf("symbols = %+v, want the section symbol", symbols)
	}
	for i, relocation := range section.Relocs {
		if relocation.Type != relocationADDR32NB || relocation.SymbolTableIndex != 0 {
			t.Errorf("relocation %d = %+v, want an image-relative address against the section symbol", i, relocation)
		}
	}
	// The root directory lists the two types in ascending order, as Windows requires.
	le := binary.LittleEndian
	if le.Uint16(data[14:]) != 2 || le.Uint32(data[16:]) != typeIcon || le.Uint32(data[24:]) != typeGroupIcon {
		t.Fatalf("root directory does not list RT_ICON then RT_GROUP_ICON")
	}
}

func TestObjectCarriesEveryImageAndNamesItInTheGroup(t *testing.T) {
	images, section, data, _ := linkable(t)
	le := binary.LittleEndian
	blob := func(i int) []byte {
		at := section.Relocs[i].VirtualAddress
		offset, size := le.Uint32(data[at:]), le.Uint32(data[at+4:])
		if uint64(offset)+uint64(size) > uint64(len(data)) {
			t.Fatalf("data entry %d points outside the section", i)
		}
		return data[offset : offset+size]
	}
	for i, image := range images {
		if !bytes.Equal(blob(i), image.Data) {
			t.Errorf("icon image %d was not copied byte for byte", i)
		}
	}
	// The group directory names every image by resource ID, in order.
	group := blob(len(images))
	if le.Uint16(group[2:]) != iconDirectoryType || int(le.Uint16(group[4:])) != len(images) {
		t.Fatalf("group directory header = % x", group[:6])
	}
	for n, image := range images {
		entry := group[6+n*groupEntrySize:]
		if int(le.Uint16(entry[12:])) != n+1 || int(le.Uint32(entry[8:])) != len(image.Data) {
			t.Errorf("group entry %d does not describe image %d", n, n+1)
		}
	}
}

func TestObjectIsDeterministic(t *testing.T) {
	ico := readFile(t, brandIcon)
	first, err := Object(ico)
	if err != nil {
		t.Fatalf("Object: %v", err)
	}
	second, err := Object(ico)
	if err != nil {
		t.Fatalf("Object: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("the same icon produced two different objects")
	}
}

func TestParseICORejectsMalformedFiles(t *testing.T) {
	valid := readFile(t, brandIcon)
	cases := map[string][]byte{
		"empty":             nil,
		"not an icon":       []byte("\x89PNG\r\n\x1a\n and more"),
		"no images":         {0, 0, 1, 0, 0, 0},
		"truncated entries": valid[:20],
		"image past end":    append(append([]byte{}, valid[:6+iconEntrySize]...), 0),
	}
	for name, data := range cases {
		if _, err := ParseICO(data); err == nil {
			t.Errorf("%s: ParseICO accepted a malformed file", name)
		}
		if _, err := Object(data); err == nil {
			t.Errorf("%s: Object accepted a malformed file", name)
		}
	}
}
