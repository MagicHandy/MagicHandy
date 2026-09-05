package web

import (
	"io/fs"
	"strings"
	"testing"
)

func TestEveryBuildIncludesOptionalLabs(t *testing.T) {
	js := readBuiltJS(t)
	for _, marker := range []string{"/api/labs/llm/chat", "/api/labs/observations", "/api/labs/tests", "/api/motion/lab/flow", "#/labs/", "LLM Lab", "Motion Lab", "Guided tests", "Observation saved."} {
		if !strings.Contains(js, marker) {
			t.Fatalf("release bundle omitted optional Labs asset %q", marker)
		}
	}
	index, err := fs.ReadFile(FS(), "index.html")
	if err != nil {
		t.Fatal(err)
	}
	mode := "release-with-optional-labs"
	if !strings.Contains(string(index), `content="`+mode+`"`) {
		t.Fatalf("UI was not built for %s mode", mode)
	}
}
