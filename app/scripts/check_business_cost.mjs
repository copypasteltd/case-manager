// Real human UI -> ordinary API -> immutable cost/source facts on native PG.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { workbenchBrowser, waitOTP } from './workbench_browser_common.mjs';
import { mobileScreen, selectMobile } from './mobile_picker_browser.mjs';
import { assertNoCredentials, recordViewport } from './project_business_recording.mjs';

const app = fileURLToPath(new URL('../', import.meta.url)), mobile = process.argv.includes('--mobile');
await workbenchBrowser(mobile, async ({ page, context, fixture, checks, evidence }) => {
  const root = () => mobile ? mobileScreen(page) : page;
  const field = label => mobile ? root().locator('input[aria-label="' + label + '"],textarea[aria-label="' + label + '"]') : page.getByLabel(label, { exact: true });
  const button = label => root().getByRole('button', { name: label, exact: true });
  let bearer;
  page.on('request', request => { if (request.headers().authorization?.startsWith('Bearer ')) bearer = request.headers().authorization; });
  await page.goto(fixture.origin);
  await field('内部账号').fill(fixture.alice.login); await field('密码').fill(fixture.alice.password);
  await button(mobile ? '登录工作空间 ↗' : '登录工作空间').click();
  await field('六位验证码').fill(await waitOTP(fixture)); await button(mobile ? '验证并继续 ↗' : '验证并继续').click();
  if (mobile) await root().locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor();
  else await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  const get = async route => {
    const response = await context.request.get((mobile ? fixture.api_origin : fixture.origin) + '/v1' + route, { headers: mobile ? { Authorization: bearer } : {} });
    assert.equal(response.status(), 200, route); return (await response.json()).data;
  };
  const navigate = route => page.goto(fixture.origin + (mobile ? '/#' : '') + route);
  const file = await get('/files/' + fixture.file_id), version = await get('/files/' + fixture.file_id + '/versions/' + fixture.file_version_id);
  const screenshots = [], created = [], prefix = 'business-cost-' + (mobile ? 'mobile' : 'web');
  await mkdir(path.join(app, '../docs/media/screenshots'), { recursive: true });
  async function shot(name) {
    await assertNoCredentials(page);
    const file = 'docs/media/screenshots/' + prefix + '-' + name + '.png';
    await page.screenshot({ path: path.join(app, '..', file) });
    screenshots.push({ file, sha256: createHash('sha256').update(await readFile(path.join(app, '..', file))).digest('hex') });
  }
  let recording;
  for (const [index, source] of fixture.business_sources.entries()) {
    const route = ({ contract: 'contracts', purchase: 'purchases', claim: 'claims' })[source.kind];
    const sourceValue = await get('/' + route + '/' + source.id);
    if (mobile) {
      // Use the product's actual Taro navigation. Directly rewriting an H5
      // hash can restore an older cached page instance with its old params.
      await navigate('/pages/home/index');
      await root().locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor();
      await root().getByText(({ contract: '合同', purchase: '采购', claim: '报销' })[source.kind], { exact: true }).click();
      await root().getByText(sourceValue.title, { exact: true }).click();
      await root().locator('.cp-title').filter({ hasText: sourceValue.title }).waitFor();
    } else {
      await navigate('/' + route + '/' + source.id);
      await page.getByRole('heading', { name: sourceValue.title, exact: true }).waitFor();
    }
    await button('登记实际费用并关联').click();
    await field(mobile ? '财务记录标题' : '费用标题').fill('实际履行费用 · ' + source.kind + ' · ' + (mobile ? '移动端' : 'Web'));
    const basis = await get('/cost-business-sources/' + source.kind + '/' + source.id);
    assert.equal(await field(mobile ? '实际原币金额' : '费用总额').inputValue(), '', 'Approved amount is never silently an incurred amount.');
    await field(mobile ? '费用发生日期' : '实际发生日').fill('2025-02-10');
    await field(mobile ? '财务类别' : '费用类别').fill('business-cost');
    await field(mobile ? '实际原币金额' : '费用总额').fill('25.01');
    await button(mobile ? '选择固定往来方' : '选择费用供应方').click();
    await root().getByRole('button', { name: new RegExp(fixture.supplier_name) }).click();
    await button(mobile ? '添加费用分摊' : '添加费用分摊').click();
    await field('分摊金额 1').fill('20');
    await button('添加费用分摊').click();
    if (mobile) await button('设为公司分摊 2').click(); else await page.getByLabel('分摊范围 2', { exact: true }).selectOption('');
    await field('分摊金额 2').fill('5.01');
    await button(mobile ? '选择财务凭证版本' : '选择费用凭证').click();
    await root().getByRole('button').filter({ hasText: file.name }).first().click();
    if (mobile) await button('使用固定版本 ' + version.seq).click();
    else await page.getByRole('button', { name: new RegExp('^版本 ' + version.seq + ' ·') }).click();
    await field(mobile ? '登记或修订原因' : '登记或修订原因').fill('已核对实际发生与固定批准来源，付款仍单独登记。');
    if (mobile) {
      await selectMobile(page, '已核对实际发生日期、金额、供应方、凭证与分摊', '已核对实际发生事实');
      await selectMobile(page, '已核对固定批准来源，未将审批金额直接当作费用', '已核对准确批准来源');
      fixture.alice.last_step = Math.floor(Date.now() / 30000);
      await field('验证器六位验证码').fill(await waitOTP(fixture)); await button('确认本人操作').click();
      await root().getByText('已通过本次两步验证，请继续核对并确认操作。', { exact: true }).waitFor();
    } else {
      await page.getByLabel('我已核对实际发生日期、金额、供应方、凭证与分摊', { exact: true }).check();
      await page.getByLabel('我已核对固定批准来源，未将审批金额直接当作实际费用', { exact: true }).check();
      await page.getByText('最近已完成两步验证。请继续核对内容，再确认本次操作。', { exact: true }).waitFor();
    }
    const submit = async () => {
      const pending = page.waitForResponse(response => new URL(response.url()).pathname === '/v1/costs/from-business' && response.request().method() === 'POST');
      void pending.catch(() => {});
      await button('确认实际费用并关联来源').click();
      const response = await pending;
      assert.equal(response.status(), 201, await response.text());
      const cost = (await response.json()).data;
      assert.equal(cost.version, 3); assert.equal(cost.state, 'confirmed'); assert.equal(cost.project_id, fixture.project_id);
      assert.equal(cost.classification, basis.classification); assert.equal(cost.amount, '25.010000'); assert.equal(cost.occurred_on, '2025-02-10');
      assert.deepEqual(cost.allocations.map(v => [v.project_id, v.amount]), [[fixture.project_id, '20.000000'], [null, '5.010000']]);
      assert.deepEqual(cost.evidence_version_ids, [fixture.file_version_id]);
      assert.equal(cost.links.length, 1); assert.equal(cost.links[0].source_id, source.id); assert.equal(cost.links[0].source_revision_id, basis.source.revision_id); assert.equal(cost.links[0].source_type, source.kind);
      if (mobile) await root().getByText('费用原额', { exact: true }).waitFor();
      else await page.getByRole('heading', { name: '准确费用事实', exact: true }).waitFor();
      const events = await get('/costs/' + cost.id + '/events?limit=20');
      assert.deepEqual([...events.items].sort((a, b) => a.version - b.version).map(v => v.kind), ['draft_created', 'confirmed', 'link_added']);
      created.push({ source: basis.source, id: cost.id, current_revision_id: cost.current_revision_id, amount: cost.amount, project_amount: '20.000000', company_amount: '5.010000' });
      if (index === 0) {
        await shot('recorded');
        if (mobile) { await root().getByText('财务操作历史', { exact: true }).scrollIntoViewIfNeeded(); await root().getByText('版本 3 · link_added', { exact: true }).waitFor(); }
        else { await button('查看登记记录').click(); await page.getByText('新增业务关联', { exact: true }).waitFor(); }
        await shot('history');
      }
    };
    if (index === 0) { await shot('review'); recording = await recordViewport(page, app, prefix, submit); }
    else await submit();
    checks.push('Actual ' + (mobile ? 'Taro H5' : 'Web') + ' ' + source.kind + ' journey records explicitly reviewed actual expense, exact source, fixed evidence and independent company/project shares.');
  }
  const all = await get('/reports/expenses?kind=incurred&category=business-cost&from=2025-02-01&to=2025-03-01');
  const project = await get('/reports/expenses?kind=incurred&category=business-cost&from=2025-02-01&to=2025-03-01&project_id=' + fixture.project_id);
  const company = await get('/reports/expenses?kind=incurred&category=business-cost&from=2025-02-01&to=2025-03-01&company_only=true');
  assert.equal(all.totals[0].net_amount, '75.030000'); assert.equal(project.totals[0].net_amount, '60.000000'); assert.equal(company.totals[0].net_amount, '15.030000');
  checks.push('Three actual expenses total75.03; project60.00/company15.03 are exact independent shares, not duplicated approved amounts.');
  assert.ok(recording && screenshots.length === 3 && created.length === 3);
  await writeFile(path.join(evidence, 'media.json'), JSON.stringify({ screenshots, recording, created, synthetic_data: true, real_model: false, real_cloud: false, mfa_recorded: false }, null, 2) + '\n');
}, 'business-cost');
