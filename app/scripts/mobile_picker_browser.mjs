// Exercise the actual Taro wheel. Never writes component or business state.
import assert from 'node:assert/strict';
export const mobileScreen = page => page.locator('.taro_page_show.taro_page_stationed:not(.taro_page_shade) .cp-page').last();
export async function stableMobile(page) { await page.evaluate(async () => { await document.fonts.ready; await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))); }); }
export async function selectMobile(page, label, text) {
  const picker = mobileScreen(page).locator('taro-picker-core[aria-label="' + label + '"]');
  await picker.click(); const overlay = page.locator('.weui-picker:visible'); await overlay.waitFor();
  const group = overlay.locator('.weui-picker__group'); await group.waitFor();
  await group.evaluate(async node => { if (node.componentOnReady) await node.componentOnReady(); });
  await overlay.evaluate(async node => { await Promise.all(node.getAnimations({ subtree: true }).map(a => a.finished.catch(() => {}))); });
  await overlay.getByText(text, { exact: true }).waitFor(); await stableMobile(page);
  const items = await overlay.locator('.weui-picker__item').allTextContents(), desired = items.indexOf(text);
  assert.ok(desired >= 0, label + ' missing option');
  const current = Number(await picker.locator('input[type="hidden"]').first().getAttribute('value')) || 0;
  const box = await group.boundingBox(), item = await overlay.locator('.weui-picker__item').first().boundingBox(); assert.ok(box && item);
  const direction = Math.sign(desired - current);
  for (let index = current; index !== desired; index += direction) {
    const x = box.x + box.width / 2, y = box.y + box.height / 2;
    await page.mouse.move(x, y); await page.mouse.down();
    for (let step = 1; step <= 5; step++) { await page.mouse.move(x, y - direction * item.height * step / 5); await stableMobile(page); }
    await page.mouse.up(); await page.waitForTimeout(350); await stableMobile(page);
  }
  await overlay.locator('.weui-picker__action').filter({ hasText: /^确定$/ }).click(); await overlay.waitFor({ state: 'hidden' }); await stableMobile(page);
  assert.equal((await picker.locator('.cp-picker').textContent()).trim(), text, label + ' actual chosen value');
}
