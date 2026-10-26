import assert from 'node:assert/strict';
export async function waitForAPI(config) {
  assert.equal(config.CP_ENV, 'development');
  const health = new URL('http://' + config.CP_HTTP_ADDR + '/health/ready');
  assert.equal(health.hostname, '127.0.0.1', 'Browser checks require the owned local API');
  const deadline = Date.now() + 60_000;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(health, { signal: AbortSignal.timeout(2000) });
      if (response.ok) return;
    } catch { /* The native Go compiler may still be starting the API. */ }
    await new Promise(resolve => setTimeout(resolve, 500));
  }
  throw new Error('Local development API did not become ready; no browser has been opened');
}
