// Additional real Taro H5/UI/API scenarios; invoked only by the owned runner.
import assert from 'node:assert/strict';

export async function projectChecks({ fixture, login, request, screen, stable, layout, checks, otp, lastSteps, accounts }) {
  assert.equal(fixture.deliverable_fixture, true);
  const alice = await login(fixture.alice), bob = accounts.bob, other = accounts.other;
  const project = await request(alice.token, '/projects/' + fixture.project_id), me = await request(alice.token, '/me');
  const reviewer = await request(bob.token, '/me');
  const until = new Date(Date.now() + 7 * 86400000).toISOString();
  const milestone = await request(alice.token, '/projects/' + project.id + '/milestones', 'POST', { title: '移动端固定交付节点', owner_id: me.user.id, due_at: until });
  const risk = await request(alice.token, '/projects/' + project.id + '/risks', 'POST', { title: '移动端跨境传输风险', description: '此数据由隔离 API 准备，验证移动端真实读取。', owner_id: me.user.id, severity: 'high', due_at: until, probability: 'medium', mitigation: '保留准确版本并核对跨境链路。' });
  const issue = await request(alice.token, '/projects/' + project.id + '/issues', 'POST', { title: '移动端资料补充问题', description: '需要补充交付说明。', owner_id: me.user.id, severity: 'medium', due_at: until, resolution: '' });
  const task = await request(alice.token, '/projects/' + project.id + '/tasks', 'POST', { title: '移动端项目讨论任务', body: '隔离 API 准备任务，评论通过真实界面创建与修订。', assignee_id: me.user.id, priority: 'normal', review_required: false });
  const predecessor = await request(alice.token, '/projects/' + project.id + '/tasks', 'POST', { title: '移动端前置任务', body: '', assignee_id: me.user.id, priority: 'normal', review_required: false });
  await request(alice.token, '/tasks/' + task.id + '/dependencies', 'POST', { expected_version: task.version, predecessor_id: predecessor.id, action: 'add' });

  async function heading(page, text) { await screen(page).locator('.cp-title').filter({ hasText: text }).waitFor(); await stable(page); }
  async function home(account) {
    for (let count = 0; count < 8; count++) {
      if (account.page.url().includes('/pages/home/index') || await screen(account.page).locator('.cp-title').filter({ hasText: '工作从这里开始' }).count()) { await heading(account.page, '工作从这里开始'); return; }
      const before = account.page.url();
      await Promise.all([account.page.waitForURL(url => url.href !== before), account.page.goBack()]);
      await stable(account.page);
    }
    throw new Error('Could not return through the actual mobile navigation stack');
  }
  async function openProject(account) {
    await home(account); const page = account.page;
    await screen(page).getByRole('button', { name: /项目 查看项目范围/ }).click();
    await screen(page).locator('.cp-row-title').getByText(project.name, { exact: true }).click();
    await heading(page, project.name);
  }
  async function select(page, label, text) {
    const picker = screen(page).locator('taro-picker-core[aria-label="' + label + '"]');
    // Taro portals the actual native-H5 picker overlay into document.body.
    // Select its visible owned overlay rather than assuming it remains under
    // the labelled picker host; interact with its real wheel and confirm.
    // The overlay wrapper has zero layout height because its children are
    // fixed-positioned; the visible fixed panel is the actionable surface.
    await picker.click(); const overlay = page.locator('.weui-picker:visible'); await overlay.waitFor();
    const group = overlay.locator('.weui-picker__group'); await group.waitFor(); await stable(page);
    const items = await overlay.locator('.weui-picker__item').allTextContents(), desired = items.indexOf(text);
    assert.ok(desired >= 0, label + ' has no visible option ' + text);
    const current = Number(await picker.locator('input[type="hidden"]').first().getAttribute('value')) || 0;
    const box = await group.boundingBox(), item = await overlay.locator('.weui-picker__item').first().boundingBox();
    assert.ok(box && item); const x = box.x + box.width / 2, y = box.y + box.height / 2;
    if (desired !== current) { await page.mouse.move(x, y); await page.mouse.down(); await page.mouse.move(x, y - (desired - current) * item.height, { steps: 8 }); await page.mouse.up(); }
    await overlay.locator('.weui-picker__action').filter({ hasText: /^确定$/ }).click();
    await overlay.waitFor({ state: 'hidden' });
    await stable(page);
    assert.equal(await picker.locator('input[type="hidden"]').first().getAttribute('value'), String(desired));
  }
  async function command(page, route, label, expected = 200, method = 'POST') {
    const [response] = await Promise.all([page.waitForResponse(value => new URL(value.url()).pathname === '/v1' + route && value.request().method() === method), screen(page).getByRole('button', { name: label, exact: true }).click()]);
    const result = await response.json(); assert.equal(response.status(), expected, route + ' / ' + (result.error?.code ?? ''));
    return result.data;
  }
  async function mfa(page, account) {
    const previous = lastSteps.get(account.login) ?? account.last_step ?? 0;
    if (Math.floor(Date.now() / 30000) <= previous) await new Promise(resolve => setTimeout(resolve, (previous + 1) * 30000 - Date.now() + 200));
    const step = Math.floor(Date.now() / 30000); lastSteps.set(account.login, step);
    await screen(page).locator('input[maxlength="6"]').fill(otp(account.mfa_secret, step));
    await command(page, '/auth/mfa/step-up', '确认本人操作');
    await screen(page).getByText('已通过本次两步验证，请继续核对并确认操作。', { exact: true }).waitFor();
  }
  async function away(page, title) {
    let writes = 0; const count = value => { if (['POST', 'PATCH'].includes(value.method()) && !new URL(value.url()).pathname.includes('/auth/')) writes++; };
    page.on('request', count);
    try { await screen(page).getByRole('button', { name: '我的', exact: true }).click(); await heading(page, '我的工作空间'); await screen(page).getByRole('button', { name: '返回上一页', exact: true }).click(); await heading(page, title); assert.equal(writes, 0, 'Page show must never send a command'); }
    finally { page.off('request', count); }
  }
  async function openDelivery(account, name) {
    const back = screen(account.page).getByRole('button', { name: '返回项目交付列表', exact: true });
    if (await back.count()) await back.click();
    else { await openProject(account); await screen(account.page).getByRole('button', { name: /项目交付与验收/ }).click(); }
    await heading(account.page, '项目交付'); await screen(account.page).locator('.cp-row-title').getByText(name, { exact: true }).click(); await heading(account.page, name);
  }

  await openProject(alice);
  for (const [entry, item] of [['里程碑', milestone], ['风险', risk], ['问题', issue]]) {
    await screen(alice.page).getByRole('button', { name: entry + ' ↗', exact: true }).click();
    await screen(alice.page).locator('.cp-row-title').getByText(item.title, { exact: true }).click(); await heading(alice.page, item.title);
    await screen(alice.page).getByText(me.user.display_name, { exact: true }).waitFor();
    if (entry === '风险') { await screen(alice.page).getByText(risk.mitigation, { exact: true }).waitFor(); await layout(alice.page, 'project-risk'); }
    await alice.page.goBack(); await alice.page.goBack(); await heading(alice.page, project.name);
  }
  await screen(alice.page).getByRole('button', { name: '项目成员 ↗', exact: true }).click(); await heading(alice.page, '项目成员'); await screen(alice.page).getByText(me.user.display_name, { exact: true }).waitFor(); await alice.page.goBack(); await heading(alice.page, project.name);
  checks.push({ name: 'API-prepared milestones risks issues and memberships are read through real scoped native-page UI with resolved names and dates', passed: true });

  await screen(alice.page).getByRole('button', { name: '项目任务 ↗', exact: true }).click(); await screen(alice.page).locator('.cp-row-title').getByText(task.title, { exact: true }).click(); await heading(alice.page, task.title);
  await screen(alice.page).getByRole('button', { name: new RegExp(predecessor.title) }).waitFor();
  await screen(alice.page).getByRole('textbox', { name: '任务评论', exact: true }).fill('第一次移动端讨论，确认准确交付。');
  await away(alice.page, task.title); assert.equal(await screen(alice.page).getByRole('textbox', { name: '任务评论', exact: true }).inputValue(), '第一次移动端讨论，确认准确交付。');
  await command(alice.page, '/tasks/' + task.id + '/comments', '发布任务评论', 201);
  await screen(alice.page).getByText('第一次移动端讨论，确认准确交付。', { exact: true }).waitFor();
  await screen(alice.page).getByRole('button', { name: '修改我的评论', exact: true }).click();
  await screen(alice.page).getByRole('textbox', { name: '修改评论文字', exact: true }).fill('修订后的移动端讨论，补齐验收准则。');
  const comments = await request(alice.token, '/tasks/' + task.id + '/comments?limit=20');
  await command(alice.page, '/comments/' + comments.items[0].id, '保存评论修订', 201, 'PATCH');
  await screen(alice.page).getByRole('button', { name: '查看评论修订历史', exact: true }).click();
  await screen(alice.page).getByText('第一次移动端讨论，确认准确交付。', { exact: true }).waitFor();
  await layout(alice.page, 'task-discussion');
  checks.push({ name: 'Real mobile task comments create and patch immutable revisions; predecessor facts and exact-version memory input survive actual page navigation', passed: true });

  await openProject(alice); await screen(alice.page).getByRole('button', { name: /项目交付与验收/ }).click(); await heading(alice.page, '项目交付');
  await layout(alice.page, 'project-deliverables');
  await screen(alice.page).getByRole('button', { name: '新建项目交付', exact: true }).click(); await heading(alice.page, '新建项目交付');
  const deliveryName = '移动端固定成果-' + 'OriginalSource'.repeat(10);
  await screen(alice.page).getByRole('textbox', { name: '交付名称', exact: true }).fill(deliveryName);
  await screen(alice.page).getByRole('textbox', { name: '验收准则', exact: true }).fill('准则A：核对初始固定原件与说明。');
  await screen(alice.page).getByRole('textbox', { name: '保存交付原因', exact: true }).fill('移动端明确创建。');
  await select(alice.page, '交付负责人', me.user.display_name); await select(alice.page, '关联里程碑', milestone.title);
  await layout(alice.page, 'deliverable-editor');
  const created = await command(alice.page, '/projects/' + project.id + '/deliverables', '创建项目交付', 201); await heading(alice.page, deliveryName);
  const oldFile = await request(alice.token, '/files/' + fixture.file_id + '/versions/' + fixture.file_version_id), currentFile = await request(alice.token, '/files/' + fixture.file_id);
  assert.notEqual(currentFile.current_version_id, oldFile.id, 'The fixture must contain a genuinely newer version');
  const latestFile = await request(alice.token, '/files/' + fixture.file_id + '/versions/' + currentFile.current_version_id);
  async function chooseFile(version) {
    // Older fixed revisions display the same name above this form. Choose
    // the actual candidate button, never a static name in historical facts.
    await screen(alice.page).getByRole('button').filter({ has: alice.page.getByText(fixture.file_name, { exact: true }) }).filter({ hasText: '选择文件后再选择准确版本' }).click();
    await screen(alice.page).getByRole('button', { name: '选择固定版本 ' + version.seq, exact: true }).click();
  }
  async function submit(version, description, hide = false) {
    // A successful edit reauthorizes and reloads the current head. Wait for
    // whichever actual submit action the refreshed state permits, rather
    // than treating a temporary loading state as an accepted deliverable.
    await screen(alice.page).getByRole('button', { name: /^(提交固定修订|提交新的交付修订)$/ }).click();
    await chooseFile(version);
    await screen(alice.page).getByRole('textbox', { name: '本次交付说明', exact: true }).fill(description);
    await screen(alice.page).getByRole('textbox', { name: '提交交付原因', exact: true }).fill('移动端明确提交此修订。');
    if (hide) {
      await away(alice.page, deliveryName);
      assert.equal(await screen(alice.page).getByRole('textbox', { name: '本次交付说明', exact: true }).inputValue(), description);
      await screen(alice.page).getByText('尚未选择正式版本。', { exact: true }).waitFor();
      assert.equal(await screen(alice.page).getByRole('button', { name: '确认提交固定修订', exact: true }).isDisabled(), true);
      await chooseFile(version);
    }
    const result = await command(alice.page, '/deliverables/' + created.id + '/submit', '确认提交固定修订', 200);
    await screen(alice.page).locator('.cp-state').filter({ hasText: '待人工验收' }).waitFor(); return result;
  }
  const first = await submit(oldFile, '第一修订固定历史原件。', true);
  const firstRevision = await request(alice.token, '/deliverables/' + created.id + '/revisions/' + first.revision_id);
  assert.equal(firstRevision.files[0].version_id, oldFile.id); assert.equal(firstRevision.acceptance_criteria, '准则A：核对初始固定原件与说明。');
  await layout(alice.page, 'deliverable-submitted');
  checks.push({ name: 'Real UI creates a named deliverable with member and milestone; fixed historical file version submission survives hide without retaining file selection or auto-posting', passed: true });

  await openDelivery(bob, deliveryName); await screen(bob.page).getByRole('button', { name: '核实并登记验收', exact: true }).click();
  await screen(bob.page).getByRole('textbox', { name: '验收说明或退回原因', exact: true }).fill('独立核实后退回，要求澄清准则。');
  assert.equal(await screen(bob.page).getByRole('button', { name: '确认退回这一修订', exact: true }).isDisabled(), true);
  await mfa(bob.page, fixture.bob); await command(bob.page, '/deliverables/' + created.id + '/decisions', '确认退回这一修订');
  await screen(bob.page).locator('.cp-state').filter({ hasText: '已退回，待修订' }).waitFor();
  await screen(alice.page).getByRole('button', { name: '刷新当前交付', exact: true }).click();
  await screen(alice.page).getByRole('button', { name: '编辑交付资料', exact: true }).click();
  await screen(alice.page).getByRole('textbox', { name: '验收准则', exact: true }).fill('准则B：核对新版资料并保留初次退回。');
  await screen(alice.page).getByRole('textbox', { name: '保存交付原因', exact: true }).fill('按独立意见修订。');
  await command(alice.page, '/deliverables/' + created.id, '保存交付资料', 200, 'PATCH');
  const second = await submit(latestFile, '第二修订固定新的正式版本。');
  assert.notEqual(second.revision_id, first.revision_id);
  const unchanged = await request(alice.token, '/deliverables/' + created.id + '/revisions/' + first.revision_id);
  assert.equal(unchanged.files[0].version_id, oldFile.id); assert.equal(unchanged.decision.decision, 'request_changes'); assert.equal(unchanged.acceptance_criteria, firstRevision.acceptance_criteria);
  checks.push({ name: 'Actual independent MFA return, UI criteria edit and resubmission preserve the original file, criteria hash and return record', passed: true });

  await screen(bob.page).getByRole('button', { name: '刷新当前交付', exact: true }).click(); await screen(bob.page).getByRole('button', { name: '核实并登记验收', exact: true }).click();
  await select(bob.page, '验收决定', '通过验收'); await screen(bob.page).getByRole('textbox', { name: '验收说明或退回原因', exact: true }).fill('独立核实全部准确版本与准则。');
  await layout(bob.page, 'deliverable-review');
  const boxes = screen(bob.page).getByRole('checkbox'); assert.equal(await boxes.count(), 2);
  for (const box of await boxes.all()) { assert.equal(await box.isChecked(), false); await box.check(); }
  await mfa(bob.page, fixture.bob); await away(bob.page, deliveryName);
  assert.equal(await screen(bob.page).getByRole('textbox', { name: '验收说明或退回原因', exact: true }).inputValue(), '独立核实全部准确版本与准则。');
  for (const box of await screen(bob.page).getByRole('checkbox').all()) assert.equal(await box.isChecked(), false);
  assert.equal(await screen(bob.page).locator('input[maxlength="6"]').inputValue(), '');
  assert.equal(await screen(bob.page).getByRole('button', { name: '确认通过这一修订', exact: true }).isDisabled(), true);
  for (const box of await screen(bob.page).getByRole('checkbox').all()) await box.check();
  await mfa(bob.page, fixture.bob);
  await openDelivery(other, deliveryName); await screen(other.page).getByRole('button', { name: '核实并登记验收', exact: true }).click();
  await select(other.page, '验收决定', '通过验收'); await screen(other.page).getByRole('textbox', { name: '验收说明或退回原因', exact: true }).fill('另一会话保留的旧版本核实输入。');
  for (const box of await screen(other.page).getByRole('checkbox').all()) await box.check(); await mfa(other.page, fixture.bob);
  await command(bob.page, '/deliverables/' + created.id + '/decisions', '确认通过这一修订');
  const accepted = await request(alice.token, '/deliverables/' + created.id); assert.equal(accepted.state, 'accepted');
  const [conflict] = await Promise.all([other.page.waitForResponse(value => value.url().endsWith('/v1/deliverables/' + created.id + '/decisions') && value.request().method() === 'POST'), screen(other.page).getByRole('button', { name: '确认通过这一修订', exact: true }).click()]);
  assert.equal(conflict.status(), 409);
  await screen(other.page).getByText('输入已保留。请先刷新资料，核对本次结果、当前权限和最新修订后再继续；不要重复创建相同资料。', { exact: true }).waitFor();
  assert.equal(await screen(other.page).getByRole('textbox', { name: '验收说明或退回原因', exact: true }).inputValue(), '另一会话保留的旧版本核实输入。');
  assert.equal(await screen(other.page).getByRole('button', { name: '确认通过这一修订', exact: true }).isDisabled(), true);
  checks.push({ name: 'Independent acceptance requires two actual unchecked attestations and fresh step-up; hide resets checks and MFA; competing real-session decision returns 409 and preserves blocked input', passed: true });

  await screen(bob.page).getByRole('button', { name: /第1次提交 ·/ }).click(); await heading(bob.page, '交付修订历史');
  await screen(bob.page).getByText('当前交付阶段：', { exact: true }).waitFor();
  await screen(bob.page).getByText(firstRevision.acceptance_criteria, { exact: true }).waitFor();
  await screen(bob.page).getByText('当时已退回', { exact: true }).waitFor(); await layout(bob.page, 'deliverable-history');
  await screen(alice.page).getByRole('button', { name: '刷新当前交付', exact: true }).click();
  const third = await submit(latestFile, '已验收后明确提交新修订，需要重新独立验收。');
  const secondHistory = await request(alice.token, '/deliverables/' + created.id + '/revisions/' + second.revision_id); assert.equal(secondHistory.decision.decision, 'accept');
  const currentProject = await request(alice.token, '/projects/' + project.id);
  await request(alice.token, '/projects/' + project.id + '/members', 'POST', { expected_version: currentProject.version, user_id: reviewer.user.id, project_role: 'viewer', action: 'set', reason: '隔离移动只读成员权限核验' });
  // Membership mutation intentionally increments this user's auth_version.
  // Verify actual revocation, then sign in through the UI with fresh MFA;
  // do not pretend the revoked session can keep browsing as a viewer.
  const revoked = await fetch(fixture.api_origin + '/v1/deliverables/' + created.id, { headers: { Authorization: 'Bearer ' + bob.token } });
  assert.equal(revoked.status, 401);
  await screen(bob.page).getByRole('button', { name: '刷新当前交付', exact: true }).click(); await heading(bob.page, '回到工作空间');
  assert.equal(await screen(bob.page).locator('.cp-title').filter({ hasText: deliveryName }).count(), 0);
  const viewer = await login(fixture.bob); await openDelivery(viewer, deliveryName);
  for (const label of ['核实并登记验收', '编辑交付资料', '提交固定修订']) assert.equal(await screen(viewer.page).getByRole('button', { name: label, exact: true }).count(), 0);
  const readonly = await request(viewer.token, '/deliverables/' + created.id); assert.equal(readonly.can_decide, false); assert.equal(readonly.state, 'submitted'); assert.equal(readonly.current_revision_id, third.revision_id);
  await screen(viewer.page).getByRole('button', { name: '返回项目交付列表', exact: true }).click(); await heading(viewer.page, '项目交付');
  assert.equal(await screen(viewer.page).getByRole('button', { name: '新建项目交付', exact: true }).count(), 0);
  checks.push({ name: 'Fixed history and old acceptance survive a new submission; membership change actually revokes the old session with 401; real MFA login as project viewer suppresses create and review despite broad roles', passed: true });

  // Hold an actual successful server response across page hide/show. No
  // business response or state is fabricated: both writes reach the real API.
  await openProject(alice); await screen(alice.page).getByRole('button', { name: /项目交付与验收/ }).click();
  await screen(alice.page).getByRole('button', { name: '新建项目交付', exact: true }).click(); await heading(alice.page, '新建项目交付');
  const navigationName = '切页期间的同一次明确创建';
  await screen(alice.page).getByRole('textbox', { name: '交付名称', exact: true }).fill(navigationName);
  await screen(alice.page).getByRole('textbox', { name: '验收准则', exact: true }).fill('只能产生一条正式草稿。');
  await screen(alice.page).getByRole('textbox', { name: '保存交付原因', exact: true }).fill('验证真实在途响应跨页面核对。');
  await select(alice.page, '交付负责人', me.user.display_name);
  const routePattern = '**/v1/projects/' + project.id + '/deliverables';
  let release, received, held = false, originalKey, originalID;
  const gate = new Promise(resolve => { release = resolve; }), reached = new Promise(resolve => { received = resolve; });
  const hold = async route => {
    if (route.request().method() !== 'POST' || held) return route.continue();
    held = true; originalKey = route.request().headers()['idempotency-key'];
    const response = await route.fetch(); assert.equal(response.status(), 201); originalID = (await response.json()).data.id; received();
    await gate; await route.fulfill({ response });
  };
  await alice.page.route(routePattern, hold);
  try {
    const initial = alice.page.waitForResponse(value => value.url().endsWith('/v1/projects/' + project.id + '/deliverables') && value.request().method() === 'POST');
    await screen(alice.page).getByRole('button', { name: '创建项目交付', exact: true }).click(); await reached;
    await away(alice.page, '新建项目交付');
    release(); await initial; await stable(alice.page);
    assert.equal(await screen(alice.page).getByRole('textbox', { name: '交付名称', exact: true }).inputValue(), navigationName);
    const [replay] = await Promise.all([alice.page.waitForResponse(value => value.url().endsWith('/v1/projects/' + project.id + '/deliverables') && value.request().method() === 'POST'), screen(alice.page).getByRole('button', { name: '创建项目交付', exact: true }).click()]);
    assert.equal(replay.status(), 201); assert.equal(replay.request().headers()['idempotency-key'], originalKey); assert.equal((await replay.json()).data.id, originalID);
    await heading(alice.page, navigationName);
    const found = await request(alice.token, '/projects/' + project.id + '/deliverables?limit=20&q=' + encodeURIComponent(navigationName)); assert.equal(found.items.length, 1); assert.equal(found.items[0].id, originalID);
  } finally { release(); await alice.page.unroute(routePattern, hold); }
  checks.push({ name: 'A real successful creation response held across page hide/show reuses the original command key on explicit reconciliation and produces exactly one deliverable', passed: true });
  assert.equal(await alice.page.evaluate(() => Object.keys(localStorage).some(key => /draft|reason|decision|access|refresh|credential/i.test(key))), false);
}
