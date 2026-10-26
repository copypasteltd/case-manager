import { Text, View } from '@tarojs/components';
import { useDidHide } from '@tarojs/taro';
import { useEffect, useRef, useState } from 'react';
import { client } from '../../platform/taro';
import { MobileError, Scope, type Schemas } from '../../platform/session';
import { go, identifier, projectChangeForm, projectSection, usePage } from '../../platform/page';
import { useCollection, useRecord } from '../../platform/collection';
import { useFormDraft } from '../../platform/form';
import { CommandNotice, DraftNotice, MFAConfirmation, useCommand } from '../../platform/command';
import { changeBudgetProof, changeRequestProof, changeWriter, currentChange, currentChangeBasis, currentChangePayload,
  currentChangeRevision, implementPayload, implementationProof, sameChangeBasis } from '../../platform/change-input';
import { SelectField, TextField } from '../business/controls';
import { PayloadSnapshot } from '../detail/snapshot';
import { Action, Busy, Check, Empty, Fact, Notice, Page, Section, State, date } from '../../ui';

type Context = { change: Schemas['Change']; project: Schemas['Project']; members: Schemas['ProjectMembers'] };
export default function ProjectChangePage({ changeID }: { changeID: unknown }) {
  const page = usePage(async scope => {
    const id = identifier(changeID), change = currentChange(await client.request('/changes/' + id, { scope }), id);
    const project = await client.request<Schemas['Project']>('/projects/' + change.project_id, { scope });
    const members = await client.request<Schemas['ProjectMembers']>('/projects/' + project.id + '/members', { scope });
    if (project.id !== change.project_id) throw new MobileError('INVALID_RESPONSE', '当前项目与变更不一致。');
    return { change, project, members };
  });
  return <Page title='项目变更资料' home><Notice error={page.error} />
    {page.loading ? <Busy /> : page.result && <ChangeWorkspace key={page.result.value.change.id + ':' + page.result.value.change.version + ':' + page.result.me.authorization_version}
      context={page.result.value} me={page.result.me} onCommitted={() => void page.reload()} />}
    <Action secondary loading={page.loading} onClick={() => void page.reload()}>重新读取项目变更</Action>
  </Page>;
}
function ChangeWorkspace({ context, me, onCommitted }: { context: Context; me: Schemas['Me']; onCommitted(): void }) {
  const { change, project, members } = context, [mode, setMode] = useState<'none' | 'prepare' | 'implement' | 'history'>('none');
  const canEdit = change.can_edit && changeWriter(project, members, me, 'change.update');
  const canPrepare = change.can_prepare_request && changeWriter(project, members, me, 'change.submit') && me.actions.includes('change.update');
  const canImplement = change.can_implement && changeWriter(project, members, me, 'change.implement') && me.actions.includes('project.update');
  return <>
    <Section title={change.revision.content.title}><Fact label='业务状态'><State value={change.state} /></Fact>
      <Fact label='固定提案'><Text>变更版本 {change.version} · 提案修订 {change.revision.seq}</Text><Text selectable className='cp-code'>{change.current_revision_id}</Text></Fact>
      <View className='cp-pad'><Action secondary onClick={() => projectSection(project.id, 'changes')}>返回项目变更列表</Action><Action secondary onClick={() => projectSection(project.id, 'budgets')}>查看项目预算历史</Action>
        {change.request_id && <Action secondary onClick={() => go('request', change.request_id!)}>查看原申请与独立审批</Action>}
      </View>
    </Section>
    <PayloadSnapshot snapshot={currentChangePayload(change)} digest={change.revision.content_hash} me={me} />
    {change.implementation && <Section title='实际实施事实'>
      <Fact label='实施时间'><Text>{date(change.implementation.created_at)}</Text></Fact><Fact label='项目版本变化'><Text>{change.implementation.before_project_version} → {change.implementation.after_project_version}</Text></Fact>
      <Fact label='实施原因'><Text className='cp-paragraph'>{change.implementation.reason}</Text></Fact><Fact label='准确批准修订'><Text className='cp-code' selectable>{change.implementation.request_revision_id}</Text></Fact>
      <Text className='cp-muted cp-small cp-paragraph'>此记录是当时实施事实；当前项目与预算请从原记录重新读取。</Text>
    </Section>}
    <Section title='变更操作与不可变历史'><View className='cp-pad'>
      {mode === 'none' ? <>
        {canEdit && <Action secondary onClick={() => projectChangeForm(project.id, change.id)}>编辑完整变更提案</Action>}
        {canPrepare && <Action disabled={!change.revision.attachments.length} onClick={() => setMode('prepare')}>准备项目变更申请</Action>}
        {canPrepare && !change.revision.attachments.length && <Text className='cp-notice'>准备申请前须在提案中选择至少一个正式文件版本。</Text>}
        {canImplement && <Action onClick={() => setMode('implement')}>核对并实施已批准变更</Action>}
        {change.state === 'approved' && !canImplement && <Text className='cp-notice'>此变更尚未实施。请核对本人实施权限、项目阶段与当前基线；过期批准不能直接沿用。</Text>}
        <Action secondary onClick={() => setMode('history')}>查看完整提案修订历史</Action>
      </> : <>
        {mode === 'prepare' && canPrepare && <PrepareChange change={change} me={me} />}
        {mode === 'implement' && canImplement && <ImplementChange context={context} me={me} onCommitted={onCommitted} />}
        {mode === 'history' && <ChangeHistory change={change} me={me} />}
        <Action secondary onClick={() => setMode('none')}>返回当前变更操作</Action>
      </>}
    </View></Section>
  </>;
}
function PrepareChange({ change, me }: { change: Schemas['Change']; me: Schemas['Me'] }) {
  const draft = useFormDraft(me, { kind: 'change-prepare', id: change.id, version: change.version, revision: change.current_revision_id, hash: change.revision.content_hash }, 'prepare');
  const [checked, setChecked] = useState(false), scope = useRef<Scope>();
  const close = () => scope.current?.close(); useDidHide(close); useEffect(() => close, []);
  const command = useCommand<Schemas['Request']>(request => { draft.forget(); go('request', request.id); }, draft.binding, async request => {
    if (!changeRequestProof(request, change, 'draft')) return false;
    const current = new Scope(); scope.current = current;
    try {
      const actual = currentChange(await client.request('/changes/' + change.id, { scope: current }), change.id);
      return actual.request_id === request.id && actual.current_revision_id === change.current_revision_id
        && actual.revision.content_hash === change.revision.content_hash && actual.lifecycle === 'draft';
    } finally { current.close(); }
  });
  const blocked = command.busy || command.blocked;
  return <><DraftNotice restored={draft.restored} /><Notice error={command.error} /><CommandNotice blocked={command.blocked} />
    <Text className='cp-notice cp-paragraph'>此动作只准备固定内容的申请草稿。随后还须核对原审批顺序，明确送审并由指定人员独立决定。</Text>
    <TextField name='change-prepare-reason' label='准备变更申请原因' value={draft.fields.reason} max={200} multiline disabled={blocked} onChange={reason => { draft.update({ reason }); setChecked(false); }} />
    <Check checked={checked} disabled={blocked} onChange={setChecked}>我已核对以上完整提案、项目预算基线与固定文件</Check>
    <Action loading={command.busy} disabled={blocked || !checked || !draft.fields.reason.trim() || !change.revision.attachments.length} onClick={() => void command.submit('/changes/' + change.id + '/submit', {
      expected_version: change.version, revision_id: change.current_revision_id, content_hash: change.revision.content_hash, reason: draft.fields.reason.trim(),
    } satisfies Schemas['ChangeSubmit'])}>保存变更申请草稿</Action>
  </>;
}
function ImplementChange({ context, me, onCommitted }: { context: Context; me: Schemas['Me']; onCommitted(): void }) {
  const { change, project } = context;
  const request = useRecord<Schemas['Request']>(change.request_id ? '/requests/' + change.request_id : null);
  const current = useRecord<Schemas['ChangeBasis']>('/projects/' + project.id + '/change-basis?currency=' + change.revision.content.currency);
  const draft = useFormDraft(me, { kind: 'change-implement', id: change.id, version: change.version, revision: change.current_revision_id, hash: change.revision.content_hash }, 'implement');
  const [scopeChecked, setScopeChecked] = useState(false), [budgetChecked, setBudgetChecked] = useState(false), [fresh, setFresh] = useState(false), [error, setError] = useState<unknown>();
  const input = useRef<Schemas['ChangeImplement']>(), proofScope = useRef<Scope>();
  const close = () => proofScope.current?.close(); useDidHide(close); useEffect(() => close, []);
  let ready = false, readError: unknown;
  if (request.value && current.value) {
    try {
      currentChangeBasis(current.value, project, change.revision.content.currency);
      if (request.value.id !== change.request_id || !changeRequestProof(request.value, change, 'approved') || !sameChangeBasis(current.value, change.revision.basis)) throw new MobileError('BASIS_CHANGED', '当前项目或预算已变化；原批准不再适用，请重新建立准确提案并审批。');
      ready = true;
    } catch (cause) { readError = cause; }
  }
  const command = useCommand<Schemas['ChangeReceipt']>(() => { draft.forget(); onCommitted(); }, draft.binding, async receipt => {
    if (!input.current || !receipt || receipt.id !== change.id || receipt.revision_id !== change.current_revision_id || receipt.version !== change.version + 1) return false;
    const scope = new Scope(); proofScope.current = scope;
    try {
      const head = currentChange(await client.request('/changes/' + change.id, { scope }), change.id);
      const actualProject = await client.request<Schemas['Project']>('/projects/' + project.id, { scope });
      const basis = currentChangeBasis(await client.request('/projects/' + project.id + '/change-basis?currency=' + change.revision.content.currency, { scope }), actualProject, change.revision.content.currency);
      let budget: Schemas['ProjectBudget'] | undefined, predecessor: Schemas['ProjectBudget'] | undefined;
      if (change.revision.target_budget_id) {
        const actual = await client.request<Schemas['ProjectBudgetListResponse']['data']>('/projects/' + project.id + '/budgets?limit=100&state=approved&currency=' + change.revision.content.currency, { scope });
        budget = actual.items.find(b => b.id === change.revision.target_budget_id);
        if (!changeBudgetProof(budget, change, 'approved')) return false;
        if (change.revision.basis.budget_id) {
          const previous = await client.request<Schemas['ProjectBudgetListResponse']['data']>('/projects/' + project.id + '/budgets?limit=100&state=superseded&currency=' + change.revision.content.currency, { scope });
          predecessor = previous.items.find(b => b.id === change.revision.basis.budget_id);
        }
      }
      return implementationProof(receipt, head, actualProject, basis, change, project, input.current, me.user.id, budget, predecessor);
    } finally { scope.close(); }
  });
  const blocked = command.busy || command.blocked;
  return <><DraftNotice restored={draft.restored} /><Notice error={request.error ?? current.error ?? readError ?? error ?? command.error} /><CommandNotice blocked={command.blocked} />
    {request.loading || current.loading ? <Busy /> : ready && <>
      <Text className='cp-notice cp-paragraph'>已重新读取准确批准申请与当前基线。实施将更新完整项目范围、日期和本币种预算，费用、合同、付款仍分别处理。</Text>
      <SelectField label='范围与日期核实' value={scopeChecked ? 'checked' : 'pending'} options={[[ 'pending', '尚未核实范围与日期' ], [ 'checked', '已核对完整范围与日期变化' ]]} disabled={blocked} onChange={v => setScopeChecked(v === 'checked')} />
      <SelectField label='预算影响核实' value={budgetChecked ? 'checked' : 'pending'} options={[[ 'pending', '尚未核实预算影响' ], [ 'checked', '已核对该币种完整预算与影响' ]]} disabled={blocked} onChange={v => setBudgetChecked(v === 'checked')} />
      <TextField name='change-implement-reason' label='明确实施变更原因' value={draft.fields.reason} max={200} multiline disabled={blocked} onChange={reason => { draft.update({ reason }); setScopeChecked(false); setBudgetChecked(false); }} />
      <MFAConfirmation disabled={blocked} onFresh={setFresh} />
      <Action loading={command.busy} disabled={blocked || !scopeChecked || !budgetChecked || !fresh || !draft.fields.reason.trim()} onClick={() => {
        try { input.current = implementPayload(change, project, current.value!, request.value!, draft.fields.reason, scopeChecked, budgetChecked, fresh); setError(undefined); void command.submit('/changes/' + change.id + '/implement', input.current); }
        catch (cause) { setError(cause); }
      }}>确认实施准确批准变更</Action>
    </>}
  </>;
}
function ChangeHistory({ change, me }: { change: Schemas['Change']; me: Schemas['Me'] }) {
  const list = useCollection<Schemas['ChangeRevision']>('/changes/' + change.id + '/revisions?limit=10'), [selected, setSelected] = useState<string>();
  const detail = useRecord<Schemas['ChangeRevision']>(selected ? '/changes/' + change.id + '/revisions/' + selected : null);
  let revision: Schemas['ChangeRevision'] | undefined, error: unknown;
  if (detail.value) {
    try { revision = currentChangeRevision(detail.value, change.id); if (revision.id !== selected) throw new MobileError('INVALID_RESPONSE', '返回了其他提案修订。'); }
    catch (cause) { error = cause; }
  }
  return <><Notice error={list.error ?? detail.error ?? error} />
    <Text className='cp-notice'>历史固定依据表示当时提案，不能据此推断当前批准或实施状态。</Text>
    {list.loading ? <Busy /> : list.items.length ? list.items.map(r => <View key={r.id} className='cp-editor-row'><Action secondary onClick={() => setSelected(r.id)}>查阅完整提案修订 {r.seq}</Action><Text className='cp-muted cp-small'>{date(r.created_at)} · {r.reason}</Text></View>) : !list.error && <Empty>没有可见提案修订。</Empty>}
    {detail.loading ? <Busy /> : revision && <><Fact label='历史准确修订'><Text selectable className='cp-code'>{revision.id}</Text><Text>提案修订 {revision.seq}</Text></Fact><PayloadSnapshot snapshot={currentChangePayload(change, revision)} digest={revision.content_hash} me={me} /></>}
    {list.next && <Action secondary loading={list.moreBusy} onClick={() => void list.more()}>加载更早完整提案修订</Action>}
    {Boolean(list.error) && <Action secondary onClick={list.reload}>重新读取提案修订历史</Action>}
  </>;
}
