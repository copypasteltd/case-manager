/* Native local styleguide review. All owned browser contexts close in finally. */
const fs = require('node:fs/promises');
const path = require('node:path');
const crypto = require('node:crypto');
const { pathToFileURL } = require('node:url');
const playwrightPath = process.env.COPYPASTE_PLAYWRIGHT || 'C:/Users/carzy/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright';
const edgePath = process.env.COPYPASTE_EDGE || 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';
const { chromium } = require(playwrightPath);
const report = { reviewed_at: new Date().toISOString(), artifact: 'docs/design/styleguide.html',
  environment: 'Windows native Edge headless / file URL / synthetic data', pages: [], interactions: [],
  failures: [], console_errors: [], sources_unchanged: false,
  cleanup: { contexts_closed: false, browser_closed: false } };
const contexts = [];
const sourcePaths = ['styleguide.html', 'styleguide.css', 'styleguide.js', 'tokens.json', 'tokens.css',
  'assets/official-logo-light.png', 'assets/official-logo-dark.png', 'review_styleguide.cjs'];
const sourceSnapshot = () => Promise.all(sourcePaths.map(async file => ({
  file, sha256: crypto.createHash('sha256').update(await fs.readFile(path.join(__dirname, file))).digest('hex')
})));
const check = (name, passed, details = {}) => {
  const result = { name, passed, ...details };
  report.interactions.push(result);
  if (!passed) report.failures.push(result);
};

(async () => {
  let browser;
  try {
    await fs.mkdir(path.join(__dirname, 'evidence'), { recursive: true });
    report.sources = await sourceSnapshot();
    report.source_sha256 = crypto.createHash('sha256').update(JSON.stringify(report.sources)).digest('hex');
    browser = await chromium.launch({ executablePath: edgePath, headless: true, timeout: 30000 });
    for (const config of [
      { name: 'styleguide-light', width: 1440, height: 1100, theme: 'light', screenshot: true },
      { name: 'styleguide-dark', width: 1440, height: 1100, theme: 'dark', screenshot: true },
      { name: 'styleguide-compact', width: 1024, height: 900, theme: 'light' },
      { name: 'styleguide-tablet', width: 768, height: 1024, theme: 'light' },
      { name: 'styleguide-mobile', width: 390, height: 844, theme: 'light', screenshot: true },
      { name: 'styleguide-mobile-dark', width: 390, height: 844, theme: 'dark' },
      { name: 'styleguide-small', width: 320, height: 844, theme: 'light' }
    ]) {
      const context = await browser.newContext({ viewport: { width: config.width, height: config.height }, reducedMotion: 'reduce' });
      contexts.push(context);
      const page = await context.newPage();
      page.on('pageerror', error => report.console_errors.push({ page: config.name, message: error.message }));
      await page.goto(pathToFileURL(path.join(__dirname, 'styleguide.html')).href, { waitUntil: 'load' });
      if (config.theme === 'dark') await page.locator('#theme-toggle').click();
      await page.evaluate(() => document.fonts.ready);
      const measurements = await page.evaluate(() => {
        const visible = node => node.getBoundingClientRect().width > 0 && node.getBoundingClientRect().height > 0;
        const unnamed = [...document.querySelectorAll('a,button,input')].filter(visible).filter(n => {
          if (n.tagName === 'INPUT') return !n.labels.length && !n.getAttribute('aria-label');
          return !n.getAttribute('aria-label') && !n.textContent.trim();
        }).map(n => n.outerHTML.slice(0, 150));
        const overflow = [...document.querySelectorAll('main *,header *,aside *')].filter(visible).filter(n => {
          const rect = n.getBoundingClientRect();
          return rect.right > innerWidth + 1 || rect.left < -1;
        }).map(n => ({ tag: n.tagName, class: typeof n.className === 'string' ? n.className : '', right: n.getBoundingClientRect().right }));
        const typography = Object.fromEntries(['body', 'h1', '.task-content h3', '.task-value strong', '.button.primary'].map(selector => {
          const css = getComputedStyle(document.querySelector(selector));
          return [selector, { family: css.fontFamily, size: parseFloat(css.fontSize), line_height: css.lineHeight }];
        }));
        return { viewport: innerWidth, document_width: document.documentElement.scrollWidth, overflow, typography,
          theme: document.documentElement.dataset.theme, canvas: getComputedStyle(document.body).backgroundColor,
          primary_background: getComputedStyle(document.querySelector('.button.primary')).backgroundColor,
          missing_images: [...document.images].filter(n => !n.complete || !n.naturalWidth).map(n => n.src), unnamed_controls: unnamed,
          mobile_target_heights: [...document.querySelectorAll('.icon-button,.mobile-nav a,.heading-actions .button,#sample-form button')].filter(visible).map(n => ({ name: n.getAttribute('aria-label') || n.textContent.trim(), height: n.getBoundingClientRect().height })),
          reduced_motion: getComputedStyle(document.querySelector('.button')).transitionDuration };
      });
      measurements.palette = await page.locator('[data-token-value]').evaluateAll(nodes => nodes.map(node => ({
        token: node.dataset.tokenValue, displayed_value: node.textContent,
        swatch_background: getComputedStyle(node.parentElement.querySelector('.swatch')).backgroundColor
      })));
      measurements.type_samples = await page.locator('.display-sample,.page-title-sample,.body-sample,.reading-sample,.money-sample').evaluateAll(nodes => nodes.map(node => {
        const style = getComputedStyle(node);
        return { sample: node.className, font_size: style.fontSize, line_height: style.lineHeight, font_family: style.fontFamily };
      }));
      const fontSession = await context.newCDPSession(page);
      try {
        await fontSession.send('DOM.enable');
        await fontSession.send('CSS.enable');
        const document = await fontSession.send('DOM.getDocument');
        measurements.platform_fonts = {};
        for (const selector of ['h1', '.task-content h3']) {
          const node = await fontSession.send('DOM.querySelector', { nodeId: document.root.nodeId, selector });
          const fonts = await fontSession.send('CSS.getPlatformFontsForNode', { nodeId: node.nodeId });
          measurements.platform_fonts[selector] = fonts.fonts;
        }
      } finally { await fontSession.detach(); }
      report.pages.push({ name: config.name, ...measurements });
      check(config.name + ': layout and assets', measurements.document_width <= config.width && !measurements.overflow.length && !measurements.missing_images.length && !measurements.unnamed_controls.length, measurements);
      check(config.name + ': theme', measurements.theme === config.theme && measurements.canvas === (config.theme === 'dark' ? 'rgb(15, 19, 26)' : 'rgb(246, 246, 244)') && measurements.primary_background === (config.theme === 'dark' ? 'rgb(232, 237, 248)' : 'rgb(21, 23, 26)'));
      check(config.name + ': reduced motion', measurements.reduced_motion === '0s');
      const typography = measurements.typography;
      check(config.name + ': work text typography', /sans-serif$/i.test(typography.body.family)
        && /serif$/i.test(typography.h1.family) && !/sans-serif$/i.test(typography.h1.family)
        && typography['.task-content h3'].family === typography.body.family
        && typography['.button.primary'].family === typography.body.family
        && typography['.task-content h3'].size >= (config.width < 640 ? 16 : 14)
        && typography['.task-value strong'].size >= 14, typography);
      if (config.width < 640) check(config.name + ': mobile core targets', measurements.mobile_target_heights.every(target => target.height >= 48));
      if (config.screenshot) {
        await page.screenshot({ path: path.join(__dirname, 'evidence', config.name + '.png'), fullPage: false });
        if (config.width < 640) {
          await page.locator('.swatches').screenshot({ path: path.join(__dirname, 'evidence', config.name + '-foundations.png'), animations: 'disabled' });
          await page.locator('.token-panel').last().evaluate(node => window.scrollTo({ top: window.scrollY + node.getBoundingClientRect().top - 16, behavior: 'instant' }));
          await page.locator('.token-panel').last().screenshot({ path: path.join(__dirname, 'evidence', config.name + '-typography.png'), animations: 'disabled' });
        } else {
          await page.locator('.foundation-grid').screenshot({ path: path.join(__dirname, 'evidence', config.name + '-foundations.png'), animations: 'disabled' });
        }
      }
      if (config.name === 'styleguide-light') {
        await page.locator('.sample-action').first().click();
        check('sample action stays local', (await page.locator('#sample-feedback').innerText()).includes('没有提交业务数据'));
        await page.locator('#sample-form button').click();
        check('empty form focus and field error', await page.evaluate(() => document.activeElement.id === 'error-summary' && document.querySelector('#sample-name').getAttribute('aria-invalid') === 'true' && !document.querySelector('#sample-name-error').hidden));
        await page.locator('#error-summary a').click();
        check('summary links to input', await page.evaluate(() => document.activeElement.id === 'sample-name'));
        await page.locator('#sample-name').fill('演示项目');
        await page.locator('#sample-form button').click();
        check('valid form stays local and clears error', await page.evaluate(() => document.querySelector('#error-summary').hidden && !document.querySelector('#sample-name').hasAttribute('aria-invalid') && document.querySelector('#sample-feedback').textContent.includes('没有创建项目')));
        await page.locator('.nav-list a[href="#files"]').click();
        check('navigation has current location', await page.locator('.nav-list a[href="#files"]').getAttribute('aria-current') === 'location');
        await page.keyboard.press('Tab');
        check('keyboard focus visible', await page.evaluate(() => { const e = document.activeElement; return e.matches(':focus-visible') && getComputedStyle(e).outlineStyle !== 'none'; }));
      }
      await context.close();
    }
    report.sources_unchanged = JSON.stringify(report.sources) === JSON.stringify(await sourceSnapshot());
    check('styleguide sources unchanged during review', report.sources_unchanged);
    if (report.console_errors.length) report.failures.push({ name: 'page runtime errors', ...report.console_errors });
  } catch (error) {
    report.failures.push({ name: 'review exception', message: error.stack });
  } finally {
    const closed = await Promise.allSettled(contexts.map(context => context.close()));
    report.cleanup.contexts_closed = closed.every(result => result.status === 'fulfilled');
    if (browser) { await browser.close(); report.cleanup.browser_closed = !browser.isConnected(); }
    await fs.writeFile(path.join(__dirname, 'evidence', 'styleguide-review.json'), JSON.stringify(report, null, 2) + '\n');
  }
  console.log(JSON.stringify({ pages: report.pages.length, checks: report.interactions.length, failures: report.failures, cleanup: report.cleanup }));
  if (report.failures.length || !report.cleanup.browser_closed || !report.cleanup.contexts_closed) process.exitCode = 1;
})();
