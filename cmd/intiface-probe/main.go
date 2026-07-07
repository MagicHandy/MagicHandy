package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/mapledaemon/MagicHandy/internal/transport/intiface"
)

func main() {
	c := intiface.NewClient(intiface.ClientOptions{ServerURL: "ws://127.0.0.1:12345"})
	ctx := context.Background()
	if err := c.Connect(ctx); err != nil {
		fmt.Println("connect err:", err)
		os.Exit(1)
	}
	devices, err := c.Scan(ctx)
	if err != nil {
		fmt.Println("scan err:", err)
	}
	b, _ := json.MarshalIndent(devices, "", "  ")
	fmt.Println("devices:", string(b))
	fmt.Println("selected:", c.SelectedDeviceID(), c.SelectedDeviceName())
}
