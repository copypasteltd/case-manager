/* Native read-only website design audit. No forms or business actions are sent.
 * Run: node docs/design/capture_official.cjs
 * All browser contexts and owned pages close in finally; no user profile is used.
 */
const fs = require('node:fs/promises');
const path = require('node:path');
const crypto = require('node:crypto');
const { chromium } = require('C:/Users/carzy/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright');

const base = __dirname;
const root = 'https://copypaste.hk';
const contexts = [];
const report = { captured_at: new Date().toISOString(), source: root, browser: 'native Microsoft Edge headless',
  pages: [], assets: [], errors: [], cleanup: { contexts_closed: false, browser_closed: false } };
const digest = data => crypto.createHash('sha256').update(data).digest('hex');

(async () => {
  await fs.mkdir(path.join(base, 'evidence'), { recursive: true });
  await fs.mkdir(path.join(base, 'assets'), { recursive: true });
  let browser;
  try {
    browser = await chromium.launch({ executablePath: 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',
      headless: true, timeout: 30000 });
    for (const config of [
      { name: 'official-home-light', theme: 'light', width: 1440, height: 1000 },
      { name: 'official-home-dark', theme: 'dark', width: 1440, height: 1000 },
      { name: 'official-home-mobile', theme: 'light', width: 390, height: 844 }
    ]) {
      const context = await browser.newContext({ viewport: { width: config.width, height: config.height },
        deviceScaleFactor: 1, colorScheme: config.theme });
      contexts.push(context);
      const page = await context.newPage();
      const response = await page.goto(root + '/', { waitUntil: 'networkidle', timeout: 45000 });
      await page.evaluate(theme => {
        localStorage.setItem('preferred-theme', theme);
        document.documentElement.dataset.theme = theme;
      }, config.theme);
      await page.waitForFunction(theme => document.documentElement.dataset.theme === theme, config.theme);
      await page.locator('.hero-copy').waitFor({ state: 'visible' });
      await page.waitForTimeout(700);
      const evidence = await page.evaluate(() => {
        const rootStyle = getComputedStyle(document.documentElement);
        const variables = {};
        for (const key of ['primary', 'secondary', 'accent', 'bg', 'bg-soft', 'surface', 'text', 'text-muted', 'border', 'shadow']) {
          variables['--color-' + key] = rootStyle.getPropertyValue('--color-' + key).trim();
        }
        const styles = {};
        for (const selector of ['body', 'h1', '.button-primary', '.button-secondary', '.metric-card', '.hero-orbit', '.site-header', '.container', '.brand-logo']) {
          const node = document.querySelector(selector);
          if (!node) continue;
          const css = getComputedStyle(node);
          styles[selector] = Object.fromEntries(['color', 'backgroundColor', 'fontFamily', 'fontSize', 'fontWeight', 'lineHeight',
            'borderRadius', 'borderColor', 'boxShadow', 'padding', 'width', 'minHeight'].map(k => [k, css[k]]));
        }
        return { variables, styles, title: document.title, viewport: { width: innerWidth, height: innerHeight },
          document_width: document.documentElement.scrollWidth,
          logo: [...document.querySelectorAll('[data-brand-logo]')].map(n => ({ src: n.currentSrc || n.src, naturalWidth: n.naturalWidth, naturalHeight: n.naturalHeight })),
          loaded_fonts: [...document.fonts].map(f => ({ family: f.family, status: f.status })) };
      });
      try {
        const client = await context.newCDPSession(page);
        await client.send('DOM.enable');
        await client.send('CSS.enable');
        const document = await client.send('DOM.getDocument');
        evidence.platform_fonts = {};
        for (const selector of ['h1', '.hero-text']) {
          const node = await client.send('DOM.querySelector', { nodeId: document.root.nodeId, selector });
          evidence.platform_fonts[selector] = await client.send('CSS.getPlatformFontsForNode', { nodeId: node.nodeId });
        }
        await client.detach();
      } catch (error) { evidence.font_audit_error = String(error); }
      await page.screenshot({ path: path.join(base, 'evidence', config.name + '.png'), animations: 'disabled' });
      report.pages.push({ name: config.name, url: page.url(), status: response.status(), ...evidence });
      await context.close();
    }
    const context = await browser.newContext();
    contexts.push(context);
    for (const item of [
      { url: '/styles.css' }, { url: '/app.js' }, { url: '/' },
      { url: '/assets/logo/CopyPaste_Light.png', save: 'official-logo-light.png' },
      { url: '/assets/logo/CopyPaste_Dark.png', save: 'official-logo-dark.png' },
      { url: '/assets/logo/CopyPaste_Light.svg', save: 'official-logo-light.svg' },
      { url: '/assets/logo/CopyPaste_Dark.svg', save: 'official-logo-dark.svg' }
    ]) {
      const response = await context.request.get(root + item.url, { timeout: 35000 });
      const data = await response.body();
      const entry = { url: response.url(), status: response.status(), size_bytes: data.length, sha256: digest(data), content_type: response.headers()['content-type'] };
      if (!response.ok()) throw new Error('Audit asset failed: ' + entry.url + ' ' + entry.status);
      if (item.save) {
        await fs.writeFile(path.join(base, 'assets', item.save), data);
        entry.local_file = 'assets/' + item.save;
      }
      if (item.url === '/styles.css') {
        const css = data.toString('utf8');
        entry.declared_font_faces = (css.match(/@font-face/g) || []).length;
        entry.declared_font_imports = (css.match(/@import/g) || []).length;
        entry.breakpoints = [...css.matchAll(/@media\s*([^\{]+)/g)].map(m => m[1].trim());
      }
      report.assets.push(entry);
    }
    await context.close();
  } catch (error) {
    report.errors.push(String(error));
    process.exitCode = 1;
  } finally {
    await Promise.allSettled(contexts.map(context => context.close()));
    report.cleanup.contexts_closed = true;
    if (browser) {
      await browser.close();
      report.cleanup.browser_closed = true;
    }
    await fs.writeFile(path.join(base, 'evidence', 'website-reference.json'), JSON.stringify(report, null, 2) + '\n');
  }
  console.log(JSON.stringify({ screenshots: report.pages.map(p => p.name), assets: report.assets.length,
    cleanup: report.cleanup, errors: report.errors }));
})();
