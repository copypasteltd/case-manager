import assert from 'node:assert/strict';
import { test } from 'node:test';
import { createServer } from 'node:http';
import { once } from 'node:events';
import { randomUUID } from 'node:crypto';
import type { RunBinding } from './codex-adapter.js';
import { HTTPRuntimeBoundary } from './http-boundary.js';

const binding = (): RunBinding => ({ id: randomUUID(), authority_stamp: 'a'.repeat(64), skill_sha256: 'b'.repeat(64),
  expires_at: new Date(Date.now() + 60000).toISOString(), task_credential: 'c'.repeat(96), input: { request: '汇总当前项目' },
  prepared_context: '{}', max_input_tokens: 1000, max_output_tokens: 1000, max_tool_calls: 5 });

test('local boundary sends only a task bearer and never accepts a different run or redirects its credential', async () => {
  const run = binding(), received: { path: string; body: unknown }[] = []; let redirected = 0;
  const server = createServer(async (request, response) => {
    assert.equal(request.headers.authorization, 'Bearer ' + run.task_credential);
    let raw = ''; for await (const chunk of request) raw += chunk;
    received.push({ path: request.url!, body: JSON.parse(raw) });
    if (request.url?.endsWith('/receipts')) { response.writeHead(200, { 'Content-Type': 'application/json' }); response.end('{"receipts":[]}'); }
    else if (request.url?.endsWith('/events')) { response.writeHead(302, { Location: '/do-not-follow' }); response.end(); }
    else if (request.url === '/do-not-follow') { redirected++; response.end('{}'); }
    else { response.writeHead(200, { 'Content-Type': 'application/json' }); response.end('{"accepted":true}'); }
  });
  server.listen(0, '127.0.0.1'); await once(server, 'listening'); const address = server.address(); assert.ok(address && typeof address === 'object');
  try {
    const boundary = new HTTPRuntimeBoundary(run, `http://127.0.0.1:${address.port}/internal/v1/agent-mcp`), signal = new AbortController().signal;
    await boundary.assertCurrent(run, signal); assert.deepEqual(await boundary.receipts(run, signal), []);
    await assert.rejects(boundary.assertCurrent({ ...run, id: randomUUID() }, signal), /RUNTIME_AUTHORITY_CHANGED/);
    await assert.rejects(boundary.append(run, { type: 'run.started' }, signal), /RUNTIME_UNAVAILABLE/);
    assert.equal(redirected, 0); assert.deepEqual(received.slice(0, 2).map(item => item.body), [{}, {}]);
    assert.ok(!JSON.stringify(received).includes(run.task_credential));
  } finally { server.closeAllConnections(); await new Promise<void>(resolve => server.close(() => resolve())); }
  for (const endpoint of ['https://example.invalid/internal/v1/agent-mcp', 'http://localhost:18081/internal/v1/agent-mcp', 'http://127.0.0.1:18081/internal/v1/agent-mcp?token=hidden']) assert.throws(() => new HTTPRuntimeBoundary(run, endpoint), /RUNTIME_ENDPOINT_INVALID/);
});

test('changed sources and revoked task credentials suppress results without exposing service diagnostics', async () => {
  const run = binding(); let code = 'RUNTIME_SOURCE_CHANGED', oversized = false;
  const server = createServer((_request, response) => { response.writeHead(409, { 'Content-Type': 'application/json' });
    response.end(oversized ? 'x'.repeat(270000) : JSON.stringify({ error: { code, message: 'private-provider-diagnostic' } })); });
  server.listen(0, '127.0.0.1'); await once(server, 'listening'); const address = server.address(); assert.ok(address && typeof address === 'object');
  try {
    const boundary = new HTTPRuntimeBoundary(run, `http://127.0.0.1:${address.port}/internal/v1/agent-mcp`), signal = new AbortController().signal;
    for (code of ['RUNTIME_SOURCE_CHANGED', 'RUNTIME_PROFILE_RETIRED', 'RUNTIME_CREDENTIAL_INVALID', 'JOB_LEASE_LOST']) await assert.rejects(boundary.assertCurrent(run, signal), /^Error: RUNTIME_AUTHORITY_CHANGED$/);
    code = 'UNRECOGNIZED_PRIVATE_ERROR'; await assert.rejects(boundary.receipts(run, signal), /^Error: RUNTIME_UNAVAILABLE$/);
    oversized = true; await assert.rejects(boundary.assertCurrent(run, signal), /^Error: RUNTIME_PROTOCOL_INVALID$/);
  } finally { server.closeAllConnections(); await new Promise<void>(resolve => server.close(() => resolve())); }
});
