export interface NativeBluetoothSnapshot {
  available: boolean;
  supported: boolean;
  remembered_device: boolean;
  status: {
    connected: boolean;
    connecting: boolean;
    simulated: boolean;
    status: string;
    message: string;
    device_name?: string;
    clock_synced: boolean;
    write_bytes: number;
    playback_state: string;
  };
}
