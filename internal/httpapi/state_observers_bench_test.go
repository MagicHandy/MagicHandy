package httpapi

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// Exercise the full HTTP projection for one or several visible local tabs
// with enough catalog rows to make repeated COUNT/tag queries measurable.
func BenchmarkPopulatedStateObservers(b *testing.B) {
	settings, err := config.OpenStore(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	fake := transport.NewFake()
	s, err := New(fstest.MapFS{"index.html": {Data: []byte("<html>")}}, slog.New(slog.NewTextHandler(io.Discard, nil)), settings, Runtime{Transport: fake, MotionTransport: fake, MotionSimulated: true}, VersionInfo{Version: "bench"})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(s.Close)
	err = settings.Datastore().WithTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < 4096)
			INSERT INTO patterns(id,name,origin,kind,enabled,weight,cycle_ms,points_json,tags_json,created_at,updated_at)
			SELECT 'bench-' || i, 'Review', 'user','routine',1,1,1000,'[]','[]','','' FROM n`)
		return err
	})
	if err != nil {
		b.Fatal(err)
	}
	for _, tabs := range []int{1, 8} {
		b.Run(fmt.Sprintf("%d-tabs", tabs), func(b *testing.B) {
			requests := make([]*http.Request, tabs)
			for i := range requests {
				requests[i] = httptest.NewRequest(http.MethodGet, "/api/state", nil)
				requests[i].Header.Set(controllerHeaderName, fmt.Sprintf("observer-%d", i))
			}
			b.ReportAllocs()
			for b.Loop() {
				for _, r := range requests {
					w := httptest.NewRecorder()
					s.handleState(w, r)
					if w.Code != 200 {
						b.Fatal(w.Code)
					}
				}
			}
		})
	}
}
