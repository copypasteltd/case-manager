// Permit only the authenticated service page; capture actual pixels, without audio.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';

async function assertSafe(page) {
  assert.match(new URL(page.url()).pathname, /^\/services\/[0-9a-f-]{36}$/);
  await page.getByRole('heading', { name: 'Synthetic monitored API', exact: true }).waitFor();
  assert.equal(await page.locator('input[type="password"]:visible,input[maxlength="6"]:visible,.cp-secret:visible').count(), 0);
}

export async function recordViewport(page, app, name, perform) {
  assert.match(name, /^[a-z][a-z0-9-]+$/); await assertSafe(page);
  const directory = path.join(app, '.local/repository-showcase', name);
  await mkdir(directory, { recursive: true });
  const frames = [], started = Date.now(); let running = true, captureError, actionError;
  const capture = (async () => {
    while (running && frames.length < 900 && Date.now() - started < 120000) {
      const began = Date.now();
      try {
        await assertSafe(page);
        const file = 'frame-' + String(frames.length).padStart(6, '0') + '.jpg';
        const bytes = await page.screenshot({ path: path.join(directory, file), type: 'jpeg', quality: 90, caret: 'hide', timeout: 10000 });
        frames.push({ file, at_ms: Date.now() - started, sha256: createHash('sha256').update(bytes).digest('hex') });
      } catch (error) { captureError = error; running = false; break; }
      await new Promise(resolve => setTimeout(resolve, Math.max(20, 200 - (Date.now() - began))));
    }
  })();
  try { await new Promise(resolve => setTimeout(resolve, 600)); await perform(); await new Promise(resolve => setTimeout(resolve, 1000)); }
  catch (error) { actionError = error; }
  finally { running = false; await capture; }
  if (actionError) throw actionError; if (captureError) throw captureError;
  assert.ok(frames.length >= 5 && frames.length < 900);
  const manifest = { name, started_at: new Date(started).toISOString(), duration_ms: Date.now() - started, viewport: page.viewportSize(),
    frame_count: frames.length, frames, credentials_excluded: true, synthetic_data: true, real_browser_pixels: true, audio: false };
  const output = path.join(directory, 'frames.json'); await writeFile(output, JSON.stringify(manifest, null, 2) + '\n');
  return { name, frame_count: frames.length, duration_ms: manifest.duration_ms, viewport: manifest.viewport, credentials_excluded: true,
    manifest: path.relative(app, output).replaceAll('\\', '/'), manifest_sha256: createHash('sha256').update(await readFile(output)).digest('hex') };
}
