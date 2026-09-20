import { decodeHandyRPCMessage, encodeHandyRequest } from "./handy-ble-codec";

export interface BluetoothCharacteristicLike extends EventTarget {
  properties?: { write?: boolean; writeWithoutResponse?: boolean };
  value?: DataView;
  startNotifications?: () => Promise<BluetoothCharacteristicLike>;
  writeValue?: (bytes: Uint8Array) => Promise<void>;
  writeValueWithResponse?: (bytes: Uint8Array) => Promise<void>;
  writeValueWithoutResponse?: (bytes: Uint8Array) => Promise<void>;
}

interface PendingResponse {
  resolve: (value: Record<string, unknown>) => void;
  reject: (reason: Error) => void;
}

interface RequestOptions {
  waitForResponse?: boolean;
  // Checked at the write boundary, after queued work has completed.
  check?: () => void;
}

const RESPONSE_TIMEOUT_MS = 5000;
const WRITE_TIMEOUT_MS = 1000;
const MAX_SUBMITTED_REQUESTS = 64;

// One immutable GATT pair per connection. Retired writes/notifications can never
// find a replacement characteristic, and a stuck native Promise cannot hold a
// later session's write queue. This owns delivery only, never motion planning.
export class HandyBleSession {
  private readonly lifetime = new AbortController();
  private readonly pending = new Map<number, PendingResponse>();
  private readonly submitted = new Map<number, number>();
  private writeTail: Promise<unknown> = Promise.resolve();
  private nextID = 0;
  private failure?: Error;

  constructor(
    private readonly tx: BluetoothCharacteristicLike,
    private readonly rx: BluetoothCharacteristicLike,
    private readonly onNotification: (notification: Record<string, unknown>) => void,
    private readonly onFailure: (error: Error) => void,
  ) {
    rx.addEventListener("characteristicvaluechanged", this.handleMessage);
  }

  get active() { return !this.lifetime.signal.aborted; }

  close(message = "Bluetooth disconnected.") {
    if (!this.active) return;
    this.rx.removeEventListener("characteristicvaluechanged", this.handleMessage);
    this.lifetime.abort(new Error(message));
    this.pending.clear();
    this.submitted.clear();
  }

  async request(path: string, body: Record<string, unknown> | (() => Record<string, unknown>) = {}, options: RequestOptions = {}): Promise<Record<string, unknown>> {
    this.checkWritable(path);
    this.nextID = this.nextID >= 0xffffffff ? 1 : this.nextID + 1;
    const id = this.nextID;
    const waitForResponse = options.waitForResponse !== false;
    let response: Promise<Record<string, unknown>> | undefined;
    const write = this.writeTail.then(async () => {
      this.checkWritable(path);
      options.check?.();
      // In particular, timestamp Play only when it reaches the native writer.
      const bytes = encodeHandyRequest(path, typeof body === "function" ? body() : body, id);
      if (bytes.length > 512) throw new Error(`Bluetooth command is too large (${bytes.length} bytes).`);
      if (waitForResponse) {
        response = this.bounded(new Promise<Record<string, unknown>>((resolve, reject) => {
          this.pending.set(id, { resolve, reject });
        }), RESPONSE_TIMEOUT_MS, `Bluetooth response timed out for ${path}.`);
        // Attach immediately: a hung write can outlive a response rejection.
        void response.catch(() => undefined);
      } else {
        this.submitted.set(id, performance.now());
        if (this.submitted.size > MAX_SUBMITTED_REQUESTS) this.submitted.delete(this.submitted.keys().next().value!);
      }
      try {
        await this.bounded(this.write(bytes), WRITE_TIMEOUT_MS, "Bluetooth write timed out. Reconnect the device.");
        this.checkActive();
      } catch (error) {
        if (this.active) this.fail(asError(error));
        throw error;
      }
    });
    this.writeTail = write.catch(() => undefined);
    try {
      await write;
      return response ? await response : { ok: true, response_pending: true };
    } finally {
      this.pending.delete(id);
    }
  }

  private async write(bytes: Uint8Array) {
    const characteristic = this.tx;
    if (characteristic.properties?.write !== false && characteristic.writeValueWithResponse) {
      await characteristic.writeValueWithResponse(bytes);
    } else if (characteristic.properties?.write !== false && characteristic.writeValue) {
      await characteristic.writeValue(bytes);
    } else if (characteristic.properties?.writeWithoutResponse !== false && characteristic.writeValueWithoutResponse) {
      await characteristic.writeValueWithoutResponse(bytes);
      // Preserve the conservative radio pacing for write-without-response links.
      await this.bounded(new Promise<void>((resolve) => window.setTimeout(resolve, 20)), 100, "Bluetooth write settling timed out.");
    } else {
      throw new Error("Bluetooth TX characteristic does not support writes.");
    }
  }

  private bounded<T>(promise: Promise<T>, timeout: number, message: string): Promise<T> {
    return new Promise<T>((resolve, reject) => {
      const signal = this.lifetime.signal;
      const abort = () => finish(() => reject(signal.reason));
      const timer = window.setTimeout(() => finish(() => reject(new Error(message))), timeout);
      const finish = (action: () => void) => {
        window.clearTimeout(timer);
        signal.removeEventListener("abort", abort);
        action();
      };
      signal.addEventListener("abort", abort, { once: true });
      if (signal.aborted) abort();
      promise.then((value) => finish(() => resolve(value)), (error) => finish(() => reject(error)));
    });
  }

  private checkActive() {
    if (!this.active) throw this.lifetime.signal.reason;
  }

  private checkWritable(path: string) {
    this.checkActive();
    if (this.failure && path !== "hsp/stop") throw this.failure;
  }

  private fail(error: Error) {
    if (this.failure) return;
    // Fence motion immediately, but leave the last-resort Stop writable until
    // the owner finishes its bounded disconnect. Native writes cannot be canceled.
    this.failure = error;
    this.onFailure(error);
  }

  private readonly handleMessage = (event: Event) => {
    if (!this.active || event.target !== this.rx || !this.rx.value) return;
    const view = this.rx.value;
    try {
      const parsed = decodeHandyRPCMessage(new Uint8Array(view.buffer, view.byteOffset, view.byteLength));
      if (parsed.type === "notification") {
        this.onNotification(parsed.notification as Record<string, unknown>);
        return;
      }
      if (parsed.type !== "response") return;
      const response = parsed.response as Record<string, unknown>;
      const id = Number(response.id);
      const pending = this.pending.get(id);
      const submittedAt = this.submitted.get(id);
      this.submitted.delete(id);
      if (!pending && (submittedAt === undefined || performance.now() - submittedAt > RESPONSE_TIMEOUT_MS)) return;
      const detail = response.error as { code?: number; message?: string } | undefined;
      const error = response.ok === false || detail ? new Error(detail?.message || `Handy Bluetooth rejected the request (code ${detail?.code ?? "unknown"}).`) : undefined;
      if (pending) {
        this.pending.delete(id);
        if (error) pending.reject(error);
        else pending.resolve(response);
      } else if (error) {
        // Write-ack is not firmware acceptance. A correlated late rejection
        // invalidates the channel instead of showing an error while streaming.
        this.fail(error);
      }
    } catch (error) {
      // Unknown notification extensions are skipped by the codec. Malformed
      // supported messages cannot establish successful delivery.
      this.fail(asError(error));
    }
  };
}

function asError(error: unknown): Error { return error instanceof Error ? error : new Error(String(error)); }
