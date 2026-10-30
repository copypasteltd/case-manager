// Exercise normal authenticated six-module navigation/creation against native PG.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { workbenchBrowser, waitOTP } from './workbench_browser_common.mjs';
import { mobileScreen } from './mobile_picker_browser.mjs';
import { assertNoCredentials, recordViewport } from './project_business_recording.mjs';
const app = fileURLToPath(new URL('../', import.meta.url)), mobile = process.argv.includes('--mobile');
await workbenchBrowser(mobile, async ({ page, context, fixture, checks, evidence }) => {
  const root = () => mobile ? mobileScreen(page) : page;
  const field = label => mobile ? root().locator('input[aria-label="' + label + '"],textarea[aria-label="' + label + '"]') : page.getByLabel(label, { exact: true });
  const button = label => root().getByRole('button', { name: label, exact: true });
  let bearer; page.on('request', r => { if (r.headers().authorization?.startsWith('Bearer ')) bearer = r.headers().authorization; });
  await page.goto(fixture.origin); await field('内部账号').fill(fixture.alice.login); await field('密码').fill(fixture.alice.password);
  await button(mobile ? '登录工作空间 ↗' : '登录工作空间').click(); await field('六位验证码').fill(await waitOTP(fixture)); await button(mobile ? '验证并继续 ↗' : '验证并继续').click();
  if (mobile) await root().locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor(); else await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  const get = async route => { const response = await context.request.get((mobile ? fixture.api_origin : fixture.origin) + '/v1' + route, { headers: mobile ? { Authorization: bearer } : {} }); assert.equal(response.status(), 200, route); return (await response.json()).data; };
  const project = await get('/projects/' + fixture.project_id);
  const navigate = route => page.goto(fixture.origin + (mobile ? '/#' : '') + route);
  const screenshots = [], created = [], prefix = 'project-business-' + (mobile ? 'mobile' : 'web'), directory = path.join(app, '../docs/media/screenshots'); await mkdir(directory, { recursive: true });
  async function shot(name) { await assertNoCredentials(page); const file = 'docs/media/screenshots/' + prefix + '-' + name + '.png'; await page.screenshot({ path: path.join(app, '..', file) }); screenshots.push({ file, sha256: createHash('sha256').update(await readFile(path.join(app, '..', file))).digest('hex') }); }
  const home = mobile ? '/pages/project/index?id=' + project.id + '&section=overview' : '/projects/' + project.id + '?tab=overview';
  await navigate(home); await root().getByText('项目业务办理', { exact: true }).waitFor();
  await shot('hub');
  const recording = await recordViewport(page, app, prefix, async () => {
    const kinds = [
      ['contracts', '合同', 'contract', '合同标题', '合同标题', '保存合同草稿'],
      ['purchases', '采购', 'purchase', '采购标题', '采购标题', '保存采购草稿'],
      ['claims', '报销', 'claim', '报销标题', '报销标题', '保存报销草稿'],
      ['costs', '实际费用', 'cost', '费用标题', '财务记录标题', '保存费用草稿'],
      ['payments', '实际资金', 'payment', '资金记录标题', '财务记录标题', '保存资金草稿'],
      ['subscriptions', '订阅', 'subscription', '订阅产品', '订阅产品', '保存订阅资料'],
    ];
    for (const [kind, label, singular, webField, mobileField, save] of kinds) {
      await navigate(home); await root().getByText('项目业务办理', { exact: true }).waitFor();
      if (mobile) await button('起草项目' + (kind === 'payments' ? '实际资金' : label)).click(); else await page.getByRole('link', { name: '起草项目' + label, exact: true }).click();
      const title = '项目闭环 · ' + label + ' · ' + (mobile ? '移动端' : 'Web'); await field(mobile ? mobileField : webField).fill(title);
      if (!mobile) { assert.equal(await page.getByLabel('所属项目', { exact: true }).inputValue(), project.id); assert.equal(await page.getByLabel('所属项目', { exact: true }).isDisabled(), true); }
      else assert.equal(await button('改为公司事项').count() + await button('选择关联项目').count() + await button('选择所属项目').count(), 0);
      if (mobile && ['costs', 'payments'].includes(kind)) await field('登记或修订原因').fill('项目内明确保存的待核实草稿');
      else if (kind === 'subscriptions') await field('登记或修订依据').fill('项目内明确保存的订阅草稿');
      else if (!mobile && ['costs', 'payments'].includes(kind)) await field('登记或修订原因').fill('项目内明确保存的待核实草稿');
      else await field('创建依据').fill('项目内明确保存的业务草稿');
      const posted = page.waitForResponse(r => new URL(r.url()).pathname === '/v1/' + kind && r.request().method() === 'POST'); void posted.catch(() => {});
      await button(mobile && ['costs', 'payments'].includes(kind) ? '保存财务草稿' : save).click(); const response = await posted; assert.equal(response.status(), 201, kind + ': ' + await response.text()); const value = (await response.json()).data;
      assert.equal(value.project_id, project.id); assert.equal(value.classification, project.classification); assert.equal(value.version, 1); assert.equal(value.state ?? value.lifecycle, 'draft'); created.push({ kind, id: value.id, project_id: value.project_id, current_revision_id: value.current_revision_id });
      await navigate(mobile ? '/pages/list/index?kind=' + kind + '&project_id=' + project.id : '/' + kind + '?project_id=' + project.id);
      await root().getByText(title, { exact: true }).waitFor(); const text = await (mobile ? root() : page.locator('body')).innerText(); assert.ok(!text.includes('Other journey') && !text.includes('Company journey'));
      const list = await get('/' + kind + '?limit=20&project_id=' + project.id + (['costs', 'payments'].includes(kind) ? '&project_records=true' : '')); assert.ok(list.items.some(item => item.id === value.id));
      if (kind === 'costs') await shot('draft');
      checks.push('Actual ' + (mobile ? 'Taro H5' : 'Web') + ' project ' + kind + ' creation retains scope/classification, creates an immutable draft and returns to the filtered list.');
    }
    if (!mobile) { await navigate('/subscriptions/' + fixture.subscription_id); await page.getByRole('heading', { name: '项目月开支 · 账期核对演示', exact: true }).waitFor(); }
    const overview = await get('/projects/' + project.id + '/overview'); assert.deepEqual(overview.incurred.totals, []); checks.push('Six new business drafts and the period operation do not invent incurred money or cash.');
  });
    if (mobile) {
      await navigate('/pages/subscription-period/index?id=' + fixture.period_id); await button('登记免付').click(); await field('操作依据').fill('演示账期已人工核实本期免付，保留原预测。');
      const currentStep = Math.floor(Date.now() / 30000); fixture.alice.last_step = currentStep; await field('验证器六位验证码').fill(await waitOTP(fixture)); await button('确认本人操作').click(); await root().getByText('已通过本次两步验证，请继续核对并确认操作。', { exact: true }).waitFor();
      const review = page.waitForResponse(r => new URL(r.url()).pathname === '/v1/subscription-periods/' + fixture.period_id + '/review'); void review.catch(() => {}); await button('登记免付').last().click(); assert.equal((await review).status(), 200); const period = await get('/subscription-periods/' + fixture.period_id); assert.equal(period.recorded_state, 'waived'); assert.equal(period.actual_amount, null); assert.deepEqual(period.cost_ids, []);
      await root().getByText('演示账期已人工核实本期免付，保留原预测。', { exact: true }).first().waitFor();
      checks.push('Actual mobile MFA-backed period waiver preserves the original forecast, immutable event and empty actual-cost facts.');
    }
    await shot('subscription');
  await writeFile(path.join(evidence, 'media.json'), JSON.stringify({ screenshots, recording, created, synthetic_data: true, real_model: false, real_cloud: false }, null, 2) + '\n');
}, 'project-business');
