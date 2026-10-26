import type { Wire } from './session';

export interface NativeHTTP {
  request(id: string, method: string, path: string, headers: Record<string, string>, body: string | null): Promise<{ status: number; body: string }>;
  cancel(id: string): void;
}
// Android performs TLS, timeout, redirect rejection and byte limits natively.
// Only the configured API origin is reachable through this bridge.
export function createNativeWire(bridge: NativeHTTP): Wire {
  let serial = 0;
  return input => {
    const id = String(++serial);
    return {
      promise: bridge.request(id, input.method, '/v1' + input.path, input.headers, input.body === undefined ? null : JSON.stringify(input.body))
        .then(response => ({ status: response.status, data: JSON.parse(response.body) as unknown })),
      abort: () => bridge.cancel(id),
    };
  };
}
