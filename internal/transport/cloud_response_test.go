package transport

import "testing"

func TestCloudDeviceResponseErrorParsesEmbeddedError(t *testing.T) {
	t.Parallel()
	err := cloudDeviceResponseError([]byte(`{"error":{"code":1002,"name":"DeviceTimeout","message":"Device timeout","connected":true}}`))
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if got := err.Error(); got != `cloud device error DeviceTimeout (1002): Device timeout` {
		t.Fatalf("error = %q", got)
	}
}
