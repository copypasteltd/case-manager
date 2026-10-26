import { Text, View } from '@tarojs/components';
import { useDidHide } from '@tarojs/taro';
import { useEffect, useRef, useState } from 'react';
import { client } from '../../platform/taro';
import { MobileError, Scope, type Schemas } from '../../platform/session';
import { go, identifier, projectSection, usePage } from '../../platform/page';
import { useFormDraft } from '../../platform/form';
import { CommandNotice, DraftNotice, useCommand } from '../../platform/command';
import { FixedFilePicker, type FixedFile } from '../../platform/fixed-files';
import { formID } from '../../platform/business-input';
import { changeBasisKey, changePayload, changeReceipt, changeWriter, currentChange, currentChangeBasis, currentChangeInput,
  savedChangeProof, sameChangeBasis, type ChangeInput, type ChangeLineInput } from '../../platform/change-input';
import { SelectField, TextField } from '../business/controls';
import { Action, Busy, Check, Fact, Notice, Page, Section, State } from '../../ui';

type Context = { project: Schemas['Project']; members: Schemas['ProjectMembers']; change?: Schemas['Change'] };
type Refill = { input: ChangeInput; reason: string };
export default function ProjectChangeFormPage({ projectID, changeID, currency }: { projectID: unknown; changeID?: string; currency?: string }) {
  const page = usePage(async scope => {
    const id = identifier(projectID), project = await client.request<Schemas['Project']>('/projects/' + id, { scope });
    const members = await client.request<Schemas['ProjectMembers']>('/projects/' + id + '/members', { scope });
    const change = changeID ? currentChange(await client.request('/changes/' + identifier(changeID), { scope }), changeID) : undefined;
    if (change && change.project_id !== id) throw new MobileError('INVALID_RESPONSE', '变更不属于当前项目。');
    return { project, members, change };
  });
  return <Page title={changeID ? '编辑项目变更' : '新建项目变更'} home><Notice error={page.error} />
    {page.loading ? <Busy /> : page.result && <BasisWorkspace key={page.result.value.project.id + ':' + page.result.me.authorization_version}
      initial={page.result.value} me={page.result.me} currency={currency} onReload={() => void page.reload()} />}
    <Action secondary loading={page.loading} onClick={() => void page.reload()}>刷新变更编辑资料</Action>
  </Page>;
}
function BasisWorkspace({ initial, me, currency, onReload }: { initial: Context; me: Schemas['Me']; currency?: string; onReload(): void }) {
  const [context, setContext] = useState<Context | undefined>(initial), [basis, setBasis] = useState<Schemas['ChangeBasis']>();
  const [chosen, setChosen] = useState(initial.change?.revision.content.currency ?? currency ?? ''), [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false), [refill, setRefill] = useState<Refill>(), [serial, setSerial] = useState(0);
  const scope = useRef<Scope>(), alive = useRef(true), epoch = useRef(client.epoch);
  const close = () => { alive.current = false; scope.current?.close(); };
  useDidHide(close); useEffect(() => close, []);
  async function read(copy?: Refill) {
    if (busy || !alive.current || !/^[A-Z]{3}$/.test(chosen)) return;
    setBusy(true); setError(undefined); const current = new Scope(); scope.current = current;
    try {
      const fresh = await client.request<Schemas['Me']>('/me', { scope: current });
      if (fresh.user.id !== me.user.id || fresh.authorization_version !== me.authorization_version || client.epoch !== epoch.current) {
        setContext(undefined); setBasis(undefined); setRefill(undefined); onReload(); return;
      }
      const id = initial.project.id, project = await client.request<Schemas['Project']>('/projects/' + id, { scope: current });
      const members = await client.request<Schemas['ProjectMembers']>('/projects/' + id + '/members', { scope: current });
      const change = initial.change ? currentChange(await client.request('/changes/' + initial.change.id, { scope: current }), initial.change.id) : undefined;
      if (change && change.project_id !== id) throw new MobileError('INVALID_RESPONSE', '变更不属于当前项目。');
      const actual = currentChangeBasis(await client.request('/projects/' + id + '/change-basis?currency=' + chosen, { scope: current }), project, chosen);
      if (!current.closed && alive.current) { setContext({ project, members, change }); setBasis(actual); setRefill(copy); setSerial(n => n + 1); }
    } catch (cause) { if (!current.closed && alive.current) { setError(cause); setBasis(undefined); setContext(undefined); setRefill(undefined); } }
    finally { current.close(); if (alive.current) setBusy(false); }
  }
  const writable = context && changeWriter(context.project, context.members, me, context.change ? 'change.update' : 'change.create') && (!context.change || context.change.can_edit);
  return <><Notice error={error} />{context && <>
    <Section title={context.project.name}><Fact label='项目阶段'><State value={context.project.state} /></Fact>
      <Text className='cp-muted cp-small cp-paragraph'>提案不会立即改变项目或预算。进行中范围与日期必须经独立变更审批，再由获授权人员明确实施。</Text>
      <View className='cp-pad'><Action secondary onClick={() => projectSection(context.project.id, 'changes')}>返回变更列表核对结果</Action></View>
    </Section>
    {!writable ? <Text className='cp-notice'>当前权限、项目阶段或变更状态只允许查阅。</Text> : <>
      <Section title='读取准确币种基线'><View className='cp-pad'>
        <TextField name='change-currency' label='变更币种' value={chosen} max={3} disabled={busy || Boolean(basis) || Boolean(context.change)}
          hint='填写服务器已配置的三位大写币种，例如 HKD；保存后不能修改币种。' onChange={setChosen} />
        {!basis && <Action secondary loading={busy} disabled={!/^[A-Z]{3}$/.test(chosen)} onClick={() => void read()}>读取项目与预算基线</Action>}
      </View></Section>
      {basis && <ChangeEditor key={changeBasisKey(basis) + ':' + context.change?.current_revision_id + ':' + serial} context={context} basis={basis} me={me} refill={refill} refreshing={busy} onRead={read} />}
    </>}
  </>}</>;
}
function ChangeEditor({ context, basis, me, refill, refreshing, onRead }: { context: Context; basis: Schemas['ChangeBasis']; me: Schemas['Me']; refill?: Refill; refreshing: boolean; onRead(copy: Refill): Promise<void> }) {
  const { project, change } = context;
  const draft = useFormDraft(me, { kind: change ? 'change-edit' : 'change-create', id: (change?.id ?? project.id) + ':' + basis.currency + ':' + (change?.current_revision_id ?? 'new'), version: change?.version ?? project.version, hash: changeBasisKey(basis) }, 'save', { change: refill?.input ?? currentChangeInput(basis, change?.revision) });
  const [files, setFiles] = useState<FixedFile[]>(change?.revision.attachments.map(f => ({ file_id: f.file_id, version_id: f.version_id, seq: f.seq, name: f.name, sha256: f.sha256, size_bytes: f.size_bytes })) ?? []);
  const [accepted, setAccepted] = useState(false), [checked, setChecked] = useState(false), [error, setError] = useState<unknown>(), [focus, setFocus] = useState<string>();
  const input = draft.fields.change!, submitted = useRef<Schemas['ChangeCreate'] | Schemas['ChangePatch']>(), proofScope = useRef<Scope>();
  const close = () => proofScope.current?.close(); useDidHide(close); useEffect(() => close, []);
  useEffect(() => { if (refill) draft.update({ change: refill.input, reason: refill.reason }); }, []);
  const command = useCommand<Schemas['ChangeReceipt']>(receipt => { draft.forget(); go('change', receipt.id); }, draft.binding, async receipt => {
    if (!submitted.current || !changeReceipt(receipt, change)) return false;
    const scope = new Scope(); proofScope.current = scope;
    try {
      const actual = currentChange(await client.request('/changes/' + receipt.id, { scope }), receipt.id);
      const freshProject = await client.request<Schemas['Project']>('/projects/' + project.id, { scope });
      const freshBasis = currentChangeBasis(await client.request('/projects/' + project.id + '/change-basis?currency=' + basis.currency, { scope }), freshProject, basis.currency);
      return sameChangeBasis(freshBasis, basis) && savedChangeProof(actual, receipt, submitted.current, basis, files, me.user.id, change);
    } finally { scope.close(); }
  });
  const blocked = refreshing || command.busy || command.blocked;
  function update(patch: Partial<ChangeInput>) { draft.update({ change: { ...input, ...patch } }); setChecked(false); }
  function row(id: string, patch: Partial<ChangeLineInput>) { update({ lines: input.lines.map(l => l.id === id ? { ...l, ...patch } : l) }); }
  return <>
    <Section title='本次拟采用的完整基线'><Fact label='准确项目版本'><Text>{basis.name} · {basis.project_version}</Text></Fact>
      <Fact label='原范围'><Text className='cp-paragraph'>{basis.scope || '未填写'}</Text></Fact><Fact label='原起止日期'><Text>{basis.start_date ?? '未设置'} — {basis.due_date ?? '未设置'}</Text></Fact>
      <Fact label='当前批准预算'><Text selectable>{basis.currency} {basis.budget_total}</Text><Text selectable className='cp-code'>{basis.budget_id ?? '尚无该币种已实施预算'}</Text></Fact>
      {basis.budget_lines.map((l, i) => <Fact key={i} label={l.category}><Text>{l.period_start} — {l.period_end}</Text><Text>{basis.currency} {l.amount}</Text></Fact>)}
      <View className='cp-pad'><Check checked={accepted} disabled={blocked} onChange={setAccepted}>我明确采用以上项目版本与该币种完整预算作为本次依据</Check>
        <Text className='cp-muted cp-small'>重新读取后需再次明确采用。保留的文字不代表原批准可继续沿用。</Text>
        <Action secondary loading={refreshing} disabled={command.busy} onClick={() => void onRead({ input, reason: draft.fields.reason })}>重新读取并核对基线</Action>
      </View>
    </Section>
    <Section title='完整变更提案'><View className='cp-pad'>
      <DraftNotice restored={draft.restored} /><Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} />
      {!draft.retained && <Text className='cp-notice'>文字超过会话共享草稿预算，离开页面可能无法恢复，请妥善保留输入。</Text>}
      <TextField name='change-title' label='变更名称' value={input.title} disabled={blocked} onChange={title => update({ title })} />
      <TextField name='change-category' label='变更类别' value={input.category} disabled={blocked} onChange={category => update({ category })} />
      <TextField name='change-scope' label='完整拟实施范围' value={input.scope} multiline max={20000} disabled={blocked} onChange={scope => update({ scope })} />
      <TextField name='change-scope-delta' label='范围影响说明' value={input.scopeDelta} multiline max={20000} disabled={blocked} onChange={scopeDelta => update({ scopeDelta })} />
      <TextField name='change-schedule-delta' label='时间影响说明' value={input.scheduleDelta} multiline max={20000} disabled={blocked} onChange={scheduleDelta => update({ scheduleDelta })} />
      <TextField name='change-start' label='拟定开始日期' value={input.start} max={10} disabled={blocked} hint='YYYY-MM-DD；留空明确清除该日期。' onChange={start => update({ start })} />
      <TextField name='change-due' label='拟定截止日期' value={input.due} max={10} disabled={blocked} hint='YYYY-MM-DD；留空明确清除该日期。' onChange={due => update({ due })} />
      <SelectField label='该币种预算处理' value={input.budgetMode} options={[[ 'keep', '保持当前预算' ], [ 'replace', '完整替换该币种预算' ]]} disabled={blocked} onChange={budgetMode => update({ budgetMode })} />
      {input.budgetMode === 'replace' && <>
        <Text className='cp-notice'>以下是该币种全部拟实施明细；删除全部行即明确清空。减少金额不会抵消其他明细增加的审批金额。</Text>
        {input.lines.map((l, index) => <View className='cp-editor-row' key={l.id}>
          <Text className='cp-row-title'>预算明细 {index + 1}</Text>
          <TextField name={'budget-category-' + l.id} label={'预算类别 ' + (index + 1)} value={l.category} disabled={blocked} focus={focus} onChange={category => row(l.id, { category })} />
          <TextField name={'budget-start-' + l.id} label={'预算开始日期 ' + (index + 1)} value={l.start} max={10} disabled={blocked} onChange={start => row(l.id, { start })} />
          <TextField name={'budget-end-' + l.id} label={'预算结束日期 ' + (index + 1)} value={l.end} max={10} disabled={blocked} onChange={end => row(l.id, { end })} />
          <TextField name={'budget-amount-' + l.id} label={'预算原币金额 ' + (index + 1)} value={l.amount} max={30} disabled={blocked} hint='非负十进制字符串，不使用科学计数或千位分隔。' onChange={amount => row(l.id, { amount })} />
          <Action secondary disabled={blocked} onClick={() => { update({ lines: input.lines.filter(v => v.id !== l.id) }); setFocus(undefined); }}>删除预算明细 {index + 1}</Action>
        </View>)}
        {!input.lines.length && <Text className='cp-notice'>本次提案将清空该币种预算明细，须按原流程独立批准并明确实施。</Text>}
        <Action secondary disabled={blocked || input.lines.length >= 100} onClick={() => { const id = formID(client.operationKey()); update({ lines: [...input.lines, { id, category: '', start: '', end: '', amount: '' }] }); setFocus('budget-category-' + id); }}>添加预算明细</Action>
      </>}
      <TextField name='change-save-reason' label='提案保存原因' value={draft.fields.reason} max={200} multiline disabled={blocked} onChange={reason => { draft.update({ reason }); setChecked(false); }} />
    </View></Section>
    <Section title='选择变更固定依据'><View className='cp-pad'><FixedFilePicker projectID={project.id} value={files} optional purpose='变更' disabled={blocked} onChange={value => { setFiles(value); setChecked(false); }} /></View></Section>
    <View className='cp-pad'><Check checked={checked} disabled={blocked} onChange={setChecked}>我已核对完整提案、预算处理方式、原因与准确文件版本</Check>
      <Text className='cp-muted cp-small'>草稿可暂缺附件，准备申请前至少选择一个正式版本。保存仅新增不可变提案修订。</Text>
      <Action loading={command.busy} disabled={blocked || !accepted || !checked} onClick={() => {
        try { const payload = changePayload(input, basis, project, files, draft.fields.reason, accepted, change); submitted.current = payload; setError(undefined); void command.submit(change ? '/changes/' + change.id : '/projects/' + project.id + '/changes', payload, change ? 'PATCH' : 'POST'); }
        catch (cause) { setError(cause); }
      }}>{change ? '保存新的变更修订' : '保存项目变更草稿'}</Action>
    </View>
  </>;
}
