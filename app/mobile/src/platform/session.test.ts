import { test } from 'node:test';
import assert from 'node:assert/strict';
import { MobileClient, MobileError, Scope, type WireRequest, type WireResponse } from './session';

function deferred<T>() { let resolve!: (value: T) => void; let reject!: (error: Error) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; }); return { promise, resolve, reject }; }
const success = (data: unknown, nonce = 'public-request-nonce'): WireResponse => ({ status: 200, data: { data, meta: { request_id: nonce } } });
const tokens = (name = 'first', expires = '2030-01-01T00:00:00Z') => ({ step: 'authenticated', access_token: name + '-access-secret', refresh_token: name + '-refresh-secret', expires_at: expires });
function harness() {
  const pending: { input: WireRequest; result: ReturnType<typeof deferred<WireResponse>>; aborted: boolean }[] = [];
  const client = new MobileClient(input => { const entry = { input, result: deferred<WireResponse>(), aborted: false }; pending.push(entry); return { promise: entry.result.promise, abort() { entry.aborted = true; entry.result.reject(new Error('aborted')); } }; }, 'wechat', () => Date.parse('2026-10-23T00:00:00Z'));
  return { client, pending, async login(name = 'first', expires?: string) { const promise = client.login('owned-user', 'owned-password'); pending.at(-1)!.result.resolve(success(tokens(name, expires), name + '-nonce')); await promise; } };
}
const tick = () => new Promise<void>(resolve => setImmediate(resolve));

test('password recovery is unauthenticated and sends no stored credential', async () => {
  const h = harness(), call = h.client.resetPassword('owned-single-use-reset', 'replacement-password-long');
  assert.equal(h.pending.at(-1)!.input.path, '/auth/password/reset'); assert.deepEqual(h.pending.at(-1)!.input.headers, {});
  h.pending.at(-1)!.result.resolve(success({ changed: true })); await call; assert.equal(h.client.authenticated, false);
});
test('confirmed password change clears memory credentials while a rejected password leaves the current login intact', async () => {
  const h = harness(); await h.login();
  const rejected = h.client.changePassword('wrong-current', 'replacement-password-long', h.client.operationKey());
  h.pending.at(-1)!.result.resolve({ status: 403, data: { error: { code: 'CURRENT_PASSWORD_INVALID', message: 'Review current password' } } });
  await assert.rejects(rejected, { code: 'CURRENT_PASSWORD_INVALID' }); assert.equal(h.client.authenticated, true);
  const call = h.client.changePassword('correct-current', 'replacement-password-long', h.client.operationKey());
  assert.equal(h.pending.at(-1)!.input.path, '/me/password'); assert.ok(h.pending.at(-1)!.input.headers['Idempotency-Key']);
  h.pending.at(-1)!.result.resolve(success({ changed: true })); await call; assert.equal(h.client.authenticated, false);
});
test('a late password change result cannot clear a newer login', async () => {
  const h = harness(); await h.login(); const old = h.client.changePassword('old-current', 'replacement-password-long', h.client.operationKey()), pending = h.pending.at(-1)!;
  const assertion = assert.rejects(old, { code: 'SESSION_CHANGED' }); await h.login('newer');
  pending.result.resolve(success({ changed: true })); await assertion; assert.equal(h.client.authenticated, true);
  const me = h.client.request('/me'); assert.equal(h.pending.at(-1)!.input.headers.Authorization, 'Bearer newer-access-secret'); h.pending.at(-1)!.result.resolve(success({})); await me;
});
test('device revocation keeps other device login and clears only a confirmed current device', async () => {
  const h = harness(); await h.login(); const id = 'aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee';
  await assert.rejects(h.client.revokeDevice('../unexpected', true, h.client.operationKey()), { code: 'INVALID_DEVICE' });
  const other = h.client.revokeDevice(id, false, h.client.operationKey()); h.pending.at(-1)!.result.resolve(success({ id, revoked_at: '2026-10-25T00:00:00Z' })); await other; assert.equal(h.client.authenticated, true);
  const current = h.client.revokeDevice(id, true, h.client.operationKey()); h.pending.at(-1)!.result.resolve(success({ id, revoked_at: '2026-10-25T00:00:00Z' })); await current; assert.equal(h.client.authenticated, false);
});

test('server validation exposes only bounded public field messages, without carrying response extras or arbitrary details', async () => {
  const h = harness(); await h.login();
  const pending = h.client.request('/contracts', { method: 'POST', body: {}, key: 'owned-public-intent-0001' });
  h.pending.at(-1)!.result.resolve({ status: 422, data: { error: { code: 'REQUIRED_BUSINESS_FIELD', message: 'Review input', details: [
    { field: 'currency', safe_message: 'Unsupported currency', internal_trace: 'never propagate' },
    { field: 'notes', unsafe_message: 'never propagate' }, null, { field: 'x'.repeat(201), safe_message: 'invalid' },
  ] }, meta: { request_id: 'public-query-id' }, private_trace: 'never propagate' } });
  await assert.rejects(pending, error => {
    assert.ok(error instanceof MobileError); assert.equal(error.requestID, 'public-query-id'); assert.deepEqual(error.details, [{ field: 'currency', safe_message: 'Unsupported currency' }]);
    assert.equal(JSON.stringify(error).includes('never propagate'), false); return true;
  });
});

test('content chunks use a fixed relative endpoint and validated If-Range while retaining normal session lifetime checks', async () => {
  const h = harness(); await h.login();
  const path = '/downloads/aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee/chunks?offset=0&length=12', sha = '"' + 'a'.repeat(64) + '"';
  const own = new Scope(), reading = h.client.request(path, { scope: own, ifRange: sha });
  assert.equal(h.pending.at(-1)!.input.headers['If-Range'], sha); assert.equal(h.pending.at(-1)!.input.headers.Authorization, 'Bearer first-access-secret');
  const rejected = assert.rejects(reading, { code: 'REQUEST_CANCELLED' }); own.close(); await rejected; assert.equal(h.pending.at(-1)!.aborted, true);
  const before = h.pending.length;
  await assert.rejects(h.client.request('/me', { ifRange: sha }), { code: 'INVALID_RANGE' });
  await assert.rejects(h.client.request(path, { ifRange: 'bad\r\nHeader: no' }), { code: 'INVALID_RANGE' });
  assert.equal(h.pending.length, before);
});

test('mobile login declares its channel, keeps operation IDs independent of secrets, rejects malformed login results', async () => {
  const h = harness(); await h.login();
  assert.deepEqual(h.pending[0].input.body, { login: 'owned-user', password: 'owned-password', device_type: 'wechat' });
  const a = h.client.operationKey(), b = h.client.operationKey(); assert.notEqual(a, b); assert.match(a, /^mobile:wechat:first-nonce:/); assert.doesNotMatch(a, /access-secret|refresh-secret/);
  const bad = h.client.login('owned', 'password'); h.pending.at(-1)!.result.resolve(success(null)); await assert.rejects(bad, { code: 'INVALID_RESPONSE' }); assert.equal(h.client.authenticated, false);
});
test('concurrent reads share one refresh rotation and use the new bearer only', async () => {
  const h = harness(); await h.login('first', '2026-10-23T00:00:01Z');
  const a = h.client.request('/me'), b = h.client.request('/dashboard');
  assert.equal(h.pending.length, 2); assert.equal(h.pending[1].input.path, '/auth/refresh');
  h.pending[1].result.resolve(success(tokens('rotated'), 'rotated-nonce')); await tick();
  assert.equal(h.pending.length, 4); for (const p of h.pending.slice(2)) { assert.equal(p.input.headers.Authorization, 'Bearer rotated-access-secret'); p.result.resolve(success({ ok: p.input.path })); }
  await Promise.all([a, b]);
});
test('an old refresh failing after a new login cannot clear or overwrite the new session', async () => {
  const h = harness(); await h.login('first', '2026-10-23T00:00:01Z'); const old = h.client.request('/me');
  await h.login('new'); h.pending[1].result.resolve({ status: 401, data: { error: { code: 'SESSION_REVOKED', message: 'revoked' } } });
  await assert.rejects(old, { code: 'SESSION_REVOKED' }); assert.equal(h.client.authenticated, true);
  const fresh = h.client.request('/me'); assert.equal(h.pending.at(-1)!.input.headers.Authorization, 'Bearer new-access-secret'); h.pending.at(-1)!.result.resolve(success({ current: true })); await fresh;
});
test('late reads after logout are discarded and closing a page aborts its wire request', async () => {
  const h = harness(); await h.login(); const old = h.client.request('/projects'); const logout = h.client.logout(); assert.equal(h.client.authenticated, false);
  h.pending[1].result.resolve(success({ secret: 'old-page' })); await assert.rejects(old, { code: 'SESSION_CHANGED' });
  h.pending[2].result.resolve(success({ logged_out: true })); await logout;
  await h.login('new'); const scope = new Scope(), read = h.client.request('/me', { scope }); const entry = h.pending.at(-1)!; scope.close(); await assert.rejects(read, { code: 'REQUEST_CANCELLED' }); assert.equal(entry.aborted, true);
  const count = h.pending.length; await assert.rejects(h.client.request('/me', { scope }), { code: 'REQUEST_CANCELLED' }); assert.equal(h.pending.length, count);
});
test('commands require an explicit intent key, never automatically replay on 401 or network ambiguity', async () => {
  const h = harness(); await h.login(); await assert.rejects(h.client.request('/tasks/id/transition', { method: 'POST', body: {} }), { code: 'COMMAND_KEY_REQUIRED' });
  const key = h.client.operationKey(), command = h.client.request('/tasks/id/transition', { method: 'POST', body: { reason: 'owned' }, key }); const entry = h.pending.at(-1)!;
  assert.equal(entry.input.headers['Idempotency-Key'], key); const count = h.pending.length;
  entry.result.resolve({ status: 401, data: { error: { code: 'SESSION_EXPIRED', message: 'expired' } } }); await assert.rejects(command, { code: 'SESSION_EXPIRED' }); assert.equal(h.pending.length, count);
  const uncertain = h.client.request('/requests/id/submit', { method: 'POST', key, body: {} }); h.pending.at(-1)!.result.reject(new Error('wire disconnected'));
  await assert.rejects(uncertain, (e: unknown) => e instanceof MobileError && e.code === 'RESULT_UNCONFIRMED'); assert.equal(h.pending.length, count + 1);
});
test('GET 401 renews once and a second 401 stops without a retry loop', async () => {
  const h = harness(); await h.login(); const read = h.client.request('/me');
  h.pending[1].result.resolve({ status: 401, data: { error: { code: 'SESSION_EXPIRED' } } }); await tick();
  assert.equal(h.pending[2].input.path, '/auth/refresh'); h.pending[2].result.resolve(success(tokens('rotated'))); await tick();
  h.pending[3].result.resolve({ status: 401, data: { error: { code: 'REVOKED' } } }); await assert.rejects(read, { code: 'REVOKED' }); assert.equal(h.pending.length, 4);
});
test('MFA uses the same challenge/code contract; hostile route inputs never receive a bearer', async () => {
  const h = harness(); const verify = h.client.verify('owned-challenge', 'owned-recovery', true);
  assert.deepEqual(h.pending[0].input.body, { challenge: 'owned-challenge', code: 'owned-recovery' }); assert.equal(h.pending[0].input.path, '/auth/mfa/recover');
  h.pending[0].result.resolve(success(tokens())); await verify;
  for (const path of ['https://other.example/me', '//other.example', '/me#fragment', '/me\r\nAuthorization:x', '/../private']) await assert.rejects(h.client.request(path), { code: 'INVALID_PATH' });
  assert.equal(h.pending.length, 1);
  const path='/conversations/00000000-0000-4000-8000-000000000001/context-sources/agent_finance_report/00000000-0000-4000-8000-000000000002?context_security_version=1&max_chars=4000';
  const report=h.client.request<{text:string}>(path);
  assert.equal(h.pending[1].input.path,path);assert.match(h.pending[1].input.headers.Authorization,/^Bearer /);
  h.pending[1].result.resolve(success({text:'{"month":"2025-01","view":"actual"}'}));
  assert.equal((await report).text,'{"month":"2025-01","view":"actual"}');
});
test('MFA enrollment returns a setting secret without a repeated challenge or an authenticated session', async () => {
  const h = harness(), enrolled = h.client.enroll('owned-challenge');
  h.pending[0].result.resolve(success({ step: 'mfa_enroll', mfa_secret: 'JBSWY3DPEHPK3PXP', mfa_uri: 'otpauth://totp/owned' }));
  assert.equal((await enrolled).mfa_secret, 'JBSWY3DPEHPK3PXP'); assert.equal(h.client.authenticated, false);
  const late = h.client.enroll('old-challenge'); h.client.clear(); h.pending[1].result.resolve(success({ step: 'mfa_enroll', mfa_secret: 'JBSWY3DPEHPK3PXP' })); await assert.rejects(late, { code: 'SESSION_CHANGED' });
});

test('platform login uses its compiled channel without an existing bearer and retains MFA challenge state', async () => {
  const h = harness(); await h.login(); const call = h.client.loginMiniapp('wx-synthetic-app', 'one-use-code');
  const pending = h.pending.at(-1)!; assert.equal(pending.input.path, '/auth/miniapps/login'); assert.deepEqual(pending.input.headers, {});
  assert.deepEqual(pending.input.body, { platform: 'wechat', app_id: 'wx-synthetic-app', code: 'one-use-code' });
  pending.result.resolve(success({ step: 'mfa', challenge: 'server-mfa-challenge' }));
  assert.equal((await call).step, 'mfa'); assert.equal(h.client.authenticated, false);
});
test('late platform login cannot replace a more recent password login', async () => {
  const h = harness(), old = h.client.loginMiniapp('wx-synthetic-app', 'old-code'), pending = h.pending.at(-1)!;
  const rejected = assert.rejects(old, { code: 'SESSION_CHANGED' }); await h.login('newer');
  pending.result.resolve(success(tokens('stale'))); await rejected;
  const me = h.client.request('/me'); assert.equal(h.pending.at(-1)!.input.headers.Authorization, 'Bearer newer-access-secret');
  h.pending.at(-1)!.result.resolve(success({})); await me;
});
test('Android cannot submit a mini-program login and malformed public profiles are rejected', async () => {
  let calls = 0; const android = new MobileClient(() => { calls++; throw new Error('must not call wire'); }, 'android');
  await assert.rejects(android.loginMiniapp('tt-synthetic-app', 'code'), { code: 'MINIAPP_RUNTIME_REQUIRED' }); assert.equal(calls, 0);
  const h = harness(), profiles = h.client.miniappProfiles(); h.pending.at(-1)!.result.resolve(success({ items: [{ platform: 'android', app_id: 'invalid' }] }));
  await assert.rejects(profiles, { code: 'INVALID_RESPONSE' });
});
