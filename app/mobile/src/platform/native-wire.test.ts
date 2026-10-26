import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createNativeWire, type NativeHTTP } from './native-wire';
import { MobileClient, MobileError, Scope } from './session';

function fixture() {
  const calls: { id: string; method: string; path: string; headers: Record<string, string>; body: string | null; resolve(result: { status: number; body: string }): void; reject(error: Error): void }[] = [];
  const cancelled: string[] = [];
  const bridge: NativeHTTP = {
    request(id, method, path, headers, body) { return new Promise((resolve, reject) => calls.push({ id, method, path, headers, body, resolve, reject })); },
    cancel(id) { cancelled.push(id); calls.find(call => call.id === id)?.reject(new Error('private native transport diagnostic')); },
  };
  const client = new MobileClient(createNativeWire(bridge), 'android', () => Date.parse('2026-10-26T00:00:00Z'));
  const reply = (data: unknown) => calls.at(-1)!.resolve({ status: 200, body: JSON.stringify({ data, meta: { request_id: 'native-public-nonce' } }) });
  const login = async () => { const request = client.login('owned-android-user', 'owned-password'); reply({ step: 'authenticated', access_token: 'fake-access', refresh_token: 'fake-refresh', expires_at: '2030-01-01T00:00:00Z' }); await request; };
  return { client, calls, cancelled, reply, login };
}
test('Android authentication and command intent keep their actual device and idempotency identity through the native bridge', async () => {
  const h = fixture(); await h.login();
  assert.equal(h.calls[0].path, '/v1/auth/login');
  assert.equal(JSON.parse(h.calls[0].body!).device_type, 'android');
  const key = h.client.operationKey(), command = h.client.request('/me/sessions/example/revoke', { method: 'POST', body: {}, key });
  assert.match(key, /^mobile:android:/);
  assert.equal(h.calls.at(-1)!.headers['Idempotency-Key'], key);
  assert.equal(h.calls.at(-1)!.headers.Authorization, 'Bearer fake-access');
  assert.equal(h.calls.at(-1)!.id, '2'); h.reply({ changed: true }); await command;
});
test('fixed-version read passes the exact range condition and only a relative API path', async () => {
  const h = fixture(); await h.login();
  const sha = '"' + 'a'.repeat(64) + '"';
  const reading = h.client.request('/downloads/aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee/chunks?offset=0&length=12', { ifRange: sha });
  assert.equal(h.calls.at(-1)!.headers['If-Range'], sha);
  assert.equal(h.calls.at(-1)!.body, null);
  assert.match(h.calls.at(-1)!.path, /^\/v1\/downloads\//);
  h.reply({ data_base64: 'YWJj' }); await reading;
});
test('closing a native page cancels its exact call and never automatically repeats a write', async () => {
  const h = fixture(); await h.login(); const scope = new Scope();
  const reading = h.client.request('/me', { scope });
  const rejected = assert.rejects(reading, { code: 'REQUEST_CANCELLED' }); scope.close(); await rejected;
  assert.deepEqual(h.cancelled, ['2']); assert.equal(h.calls.length, 2);
  const key = h.client.operationKey(), command = h.client.request('/me/sessions/example/revoke', { method: 'POST', body: {}, key });
  const unknown = assert.rejects(command, (error: unknown) => error instanceof MobileError && error.code === 'RESULT_UNCONFIRMED' && !JSON.stringify(error).includes('native transport'));
  h.calls.at(-1)!.reject(new Error('private native transport diagnostic')); await unknown;
  assert.equal(h.calls.length, 3);
});
test('malformed native JSON cannot become a successful business result or disclose native diagnostics', async () => {
  const h = fixture(), loggingIn = h.client.login('owned-user', 'owned-password');
  h.calls[0].resolve({ status: 200, body: '<private upstream error>' });
  await assert.rejects(loggingIn, (error: unknown) => error instanceof MobileError && error.code === 'RESULT_UNCONFIRMED' && !error.message.includes('upstream'));
  assert.equal(h.client.authenticated, false);
});
