// Select through the visible Taro wheel. Its settled transform, rather than a
// hidden input attribute, determines the actual row after each pointer gesture.
import assert from 'node:assert/strict';
import { mobileScreen, stableMobile } from './mobile_picker_browser.mjs';

export async function selectConcernMobile(page, label, text) {
  const picker = mobileScreen(page).locator('taro-picker-core[aria-label="' + label + '"]');
  await picker.click(); const overlay = page.locator('.weui-picker:visible'); await overlay.waitFor();
  const group = overlay.locator('.weui-picker__group'); await group.waitFor();
  await group.evaluate(async node => { if (node.componentOnReady) await node.componentOnReady(); });
  await overlay.getByText(text, { exact: true }).waitFor(); await stableMobile(page);
  const items = await overlay.locator('.weui-picker__item').allTextContents(), desired = items.indexOf(text);
  assert.ok(desired >= 0, label + ' actual option');
  async function selected() {
    return group.evaluate(node => {
      const height = node.querySelector('.weui-picker__item').getBoundingClientRect().height;
      const translate = new DOMMatrixReadOnly(getComputedStyle(node.querySelector('.weui-picker__content')).transform).m42;
      return Math.round(((node.getBoundingClientRect().height - height) / 2 - translate) / height);
    });
  }
  await page.waitForTimeout(400);
  for (let attempt = 0; attempt < 50 && await selected() !== desired; attempt++) {
    const current = await selected(), direction = Math.sign(desired - current);
    const box = await group.boundingBox(), item = await overlay.locator('.weui-picker__item').first().boundingBox(); assert.ok(box && item);
    const x = box.x + box.width / 2, y = box.y + box.height / 2;
    await page.mouse.move(x, y); await page.mouse.down();
    for (let step = 1; step <= 10; step++) { await page.mouse.move(x, y - direction * item.height * step / 10); await page.waitForTimeout(35); }
    await page.mouse.up(); await page.waitForTimeout(450); await stableMobile(page);
  }
  assert.equal(await selected(), desired, label + ' settled wheel row');
  await overlay.locator('.weui-picker__action').filter({ hasText: /^确定$/ }).click(); await overlay.waitFor({ state: 'hidden' }); await stableMobile(page);
  assert.equal((await picker.locator('.cp-picker').textContent()).trim(), text, label + ' selected business value');
}
