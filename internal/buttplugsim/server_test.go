package buttplugsim

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestSimulatorAnswersTheHandshakeAndListsBothDevices(t *testing.T) {
	sim, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sim.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, sim.Address(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.CloseNow() }()
	exchange := func(request string) []map[string]map[string]any {
		t.Helper()
		if err := conn.Write(ctx, websocket.MessageText, []byte(request)); err != nil {
			t.Fatal(err)
		}
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var replies []map[string]map[string]any
		if err := json.Unmarshal(data, &replies); err != nil {
			t.Fatal(err)
		}
		return replies
	}
	info := exchange(`[{"RequestServerInfo":{"Id":1,"ClientName":"test","MessageVersion":3}}]`)
	if info[0]["ServerInfo"]["MessageVersion"] != float64(3) {
		t.Fatalf("server info = %v", info)
	}
	list := exchange(`[{"RequestDeviceList":{"Id":2}}]`)
	if devices := list[0]["DeviceList"]["Devices"].([]any); len(devices) != 2 {
		t.Fatalf("devices = %v", devices)
	}
	if ok := exchange(`[{"ScalarCmd":{"Id":3,"DeviceIndex":1,"Scalars":[{"Index":0,"Scalar":0.5,"ActuatorType":"Vibrate"}]}}]`); ok[0]["Ok"]["Id"] != float64(3) {
		t.Fatalf("ScalarCmd reply = %v", ok)
	}
}
