package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

type interruptedResponseReader struct{ err error }

func (r interruptedResponseReader) Read([]byte) (int, error) { return 0, r.err }

func TestResponsesStreamPreservesCancellationWithoutPublishingDraft(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			reader := io.MultiReader(strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"uncommitted\"}\n\n"),
				interruptedResponseReader{fmt.Errorf("stream read: %w", cause)})
			raw, err := consumeResponsesStream(reader, nil)
			if raw != "" || !errors.Is(err, cause) {
				t.Fatalf("draft=%q error=%v; want empty output and %v", raw, err, cause)
			}
		})
	}
}

func TestResponsesStreamStillRedactsUnexpectedReadErrors(t *testing.T) {
	raw, err := consumeResponsesStream(interruptedResponseReader{errors.New("private transport detail")}, nil)
	var outcome *CloudError
	if raw != "" || !errors.As(err, &outcome) || outcome.Kind != "incomplete" || strings.Contains(err.Error(), "private") {
		t.Fatalf("draft=%q error=%v", raw, err)
	}
}
