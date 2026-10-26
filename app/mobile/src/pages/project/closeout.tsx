import { Text, View } from '@tarojs/components';
import Taro, { useDidHide } from '@tarojs/taro';
import { useEffect, useRef, useState } from 'react';
import { client } from '../../platform/taro';
import { Scope, MobileError, type Schemas } from '../../platform/session';
import { identifier, go, usePage } from '../../platform/page';
import { useFormDraft } from '../../platform/form';
import { CommandNotice, DraftNotice, MFAConfirmation, useCommand } from '../../platform/command';
import { FixedFilePicker, FixedFiles, type FixedFile } from '../../platform/fixed-files';
import { currentArchives, currentCloseout, closeoutLabels, closeoutWriter, projectActions, projectTransition, projectStateReceipt,
  transitionState, archivePayload, archiveReceipt, archiveProof, type ArchiveNotes, type CloseoutKind } from '../../platform/project-closeout';
import { overviewClock, overviewItemURL, overviewState } from '../../platform/project-overview';
import { SelectField, TextField } from '../business/controls';
import { Action, Busy, Empty, Fact, Notice, Page, Row, Section, State } from '../../ui';

type Preview = Schemas['ProjectCloseout'];
type Context = { basis: Preview; members: Schemas['ProjectMembers'] };
type Refill = { notes: ArchiveNotes; files: FixedFile[] };
const explanations: Record<CloseoutKind, string> = {
  tasks: '任务须完成或取消，受阻和待审核仍需处理。', milestones: '须达成或取消；错过期限不代表完成。',
  deliverables: '按固定修订与实际人工验收核对；来源不完整不能视为通过。', risks: '须关闭或转为实际问题；关联问题另行核对。',
  issues: '已解决仍须明确关闭。', changes: '批准不等于实施；未实施或依据不完整仍需处置。',
  budgets: '预算历史是参考资料，不据预算推算费用、承诺或剩余余额。', contracts: '完成或终止仍分别核对履约、补充协议与结算。',
  purchases: '完成或取消不免除费用、退款、履约与变更核对。', claims: '获批报销、实际费用与员工垫付结算分别核对。',
  costs: '核对实际费用及完整分摊的有效结算，冲销与原资金分配分别处理。', payments: '退款、冲销与实际用途分别核对，资金余额不自动表示费用欠款。',
  subscriptions: '内部取消与供应方停费分别核对，账期与实际发生仍需闭环。',
};

export default function ProjectCloseoutPage({ projectID }: { projectID: unknown }) {
  const page = usePage(async scope => {
    const id = identifier(projectID);
    const basis = currentCloseout(await client.request<Preview>('/projects/' + id + '/closeout', { scope }), id);
    const members = await client.request<Schemas['ProjectMembers']>('/projects/' + id + '/members', { scope });
    return { basis, members };
  });
  const value = page.result?.value;
  return <Page title='项目结项与归档' home>
    <Notice error={page.error} />
    {page.loading ? <Busy /> : value && <CloseoutWorkspace key={value.basis.project.id + ':' + value.basis.as_of + ':' + page.result!.me.authorization_version}
      initial={value} me={page.result!.me} onCommitted={() => void page.reload()} />}
    <Action secondary loading={page.loading} onClick={() => void page.reload()}>重新读取项目结项</Action>
  </Page>;
}

function CloseoutWorkspace({ initial, me, onCommitted }: { initial: Context; me: Schemas['Me']; onCommitted(): void }) {
  const [value, setValue] = useState<Context | undefined>(initial), [refill, setRefill] = useState<Refill>(), [serial, setSerial] = useState(0);
  const [refreshing, setRefreshing] = useState(false), [error, setError] = useState<unknown>();
  const scope = useRef<Scope>(), alive = useRef(true), epoch = useRef(client.epoch);
  const close = () => { alive.current = false; scope.current?.close(); };
  useDidHide(close); useEffect(() => close, []);
  async function reread(notes: ArchiveNotes, files: FixedFile[]) {
    if (refreshing || !alive.current) return;
    setRefreshing(true); setError(undefined); const current = new Scope(); scope.current = current;
    try {
      const freshMe = await client.request<Schemas['Me']>('/me', { scope: current });
      if (freshMe.user.id !== me.user.id || freshMe.authorization_version !== me.authorization_version || client.epoch !== epoch.current) {
        setValue(undefined); setRefill(undefined); onCommitted(); return;
      }
      const id = initial.basis.project.id;
      const basis = currentCloseout(await client.request<Preview>('/projects/' + id + '/closeout', { scope: current }), id);
      const members = await client.request<Schemas['ProjectMembers']>('/projects/' + id + '/members', { scope: current });
      if (!current.closed && alive.current) {
        setValue({ basis, members });
        setRefill(basis.visibility === 'complete' && closeoutWriter(members, freshMe) && freshMe.actions.includes('project.archive')
          ? { notes: { ...notes }, files: files.map(file => ({ ...file })) } : undefined);
        setSerial(old => old + 1);
      }
    } catch (cause) {
      if (!current.closed && alive.current) { setError(cause); setValue(undefined); setRefill(undefined); }
    } finally { current.close(); if (alive.current) setRefreshing(false); }
  }
  return <>
    <Notice error={error} />
    {value && <>
      <Section title={value.basis.project.name}>
        <Fact label='项目阶段'><State value={value.basis.project.state} /></Fact>
        <Fact label='本次核查'><Text>{overviewClock(value.basis.as_of)}</Text><Text className='cp-muted cp-small'>项目版本 {value.basis.project.version} · 仅表示当前系统规则与已录入资料，仍须人工核对实际义务。</Text></Fact>
        <View className='cp-pad'><Action secondary onClick={() => go('project', value.basis.project.id)}>返回项目资料</Action></View>
      </Section>
      <View className='cp-notice'><Text>{value.basis.visibility === 'restricted' ? '部分依据权限受限，尚未完整核实；不能将缺项当作零阻塞。'
        : value.basis.groups.some(g => (g.open_count ?? 0) > 0) ? '仍有事项需要处置。请按清单读取原记录；结项阶段需要先恢复进行。'
          : '当前完整核查未发现规则阻塞。归档前仍需复盘、分别核实业务与财务，并完成本人验证。'}</Text></View>
      <Checklist groups={value.basis.groups} projectID={value.basis.project.id} />
      <StageForm key={'stage:' + value.basis.project.version + ':' + serial} context={value} me={me} disabled={refreshing} onCommitted={onCommitted} />
      {value.basis.can_archive && closeoutWriter(value.members, me) && me.actions.includes('project.archive') &&
        <ArchiveForm key={'archive:' + value.basis.project.version + ':' + value.basis.fingerprint + ':' + serial} context={value} me={me}
          refill={refill} refreshing={refreshing} onReread={reread} onCommitted={onCommitted} />}
      <ArchiveHistory key={value.basis.project.id + ':' + serial} projectID={value.basis.project.id} />
    </>}
  </>;
}

function Checklist({ groups, projectID, historical = false }: { groups: Schemas['ProjectCloseoutGroup'][]; projectID: string; historical?: boolean }) {
  return <Section title={historical ? '归档时固定核查清单' : '十三类完整核查清单'}>
    {groups.map(group => <ChecklistGroup key={group.kind} group={group} projectID={projectID} historical={historical} />)}
  </Section>;
}
function ChecklistGroup({ group, projectID, historical }: { group: Schemas['ProjectCloseoutGroup']; projectID: string; historical: boolean }) {
  const [expanded, setExpanded] = useState((group.open_count ?? 0) > 0), [limit, setLimit] = useState(20);
  return <View className='cp-pad'>
    <Text className='cp-row-title'>{closeoutLabels[group.kind]}</Text>
    <Text className='cp-muted cp-small'>{explanations[group.kind]}</Text>
    {group.visibility === 'restricted' ? <Text className='cp-notice'>权限受限，数量与条目尚未核实。</Text> : <>
      <Text className='cp-small'>{historical ? '归档时' : '完整核查'}共 {group.count} 项 · {group.open_count ? group.open_count + ' 项待处置' : group.kind === 'budgets' ? '参考版本' : '未发现阻塞'}</Text>
      {group.items.length > 0 && <Action secondary onClick={() => setExpanded(old => !old)}>{expanded ? '收起' : '展开'}{closeoutLabels[group.kind]}清单</Action>}
      {expanded && <>
        {group.items.slice(0, limit).map(item => <Row key={item.id} title={item.title || closeoutLabels[item.kind]} subtitle={'版本 ' + item.version + ' · ' + overviewState(item.state) + ' · ' + (item.blocking ? '待处置' : '未构成阻塞')}
          onClick={() => void Taro.navigateTo({ url: overviewItemURL(projectID, item.kind, item.id) })} />)}
        <Text className='cp-muted cp-small'>已读取本类完整清单，当前展示 {Math.min(limit, group.items.length)}/{group.items.length} 项。{historical && '原记录链接按当前权限读取当前资料。'}</Text>
        {limit < group.items.length && <Action secondary onClick={() => setLimit(old => old + 20)}>继续显示本类已核查事项</Action>}
      </>}
    </>}
  </View>;
}

function StageForm({ context, me, disabled, onCommitted }: { context: Context; me: Schemas['Me']; disabled: boolean; onCommitted(): void }) {
  const { basis, members } = context, choices = projectActions(basis, members, me);
  const draft = useFormDraft(me, { kind: 'project-closeout-transition', id: basis.project.id, version: basis.project.version }, choices[0]?.action ?? '');
  const selected = choices.find(choice => choice.action === draft.fields.action), [error, setError] = useState<unknown>();
  const command = useCommand<Schemas['Project']>(() => { draft.forget(); onCommitted(); }, draft.binding,
    value => Boolean(selected && projectStateReceipt(value, basis.project, transitionState(selected.action))));
  const blocked = disabled || command.busy || command.blocked;
  if (!choices.length) return null;
  return <Section title='项目阶段操作'><View className='cp-pad'>
    <DraftNotice restored={draft.restored} /><Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} />
    <Text className='cp-muted cp-small'>进入结项后普通业务只读，仍须另行复盘归档。暂停、取消或重开均不自动结清费用、终止合同或取消订阅。</Text>
    <SelectField label='项目阶段动作' value={draft.fields.action} options={choices.map(choice => [choice.action, choice.label] as const)} disabled={blocked} onChange={action => draft.update({ action })} />
    <TextField name='closeout-stage-reason' label='项目阶段原因' value={draft.fields.reason} max={200} multiline disabled={blocked} onChange={reason => draft.update({ reason })} />
    <Action loading={command.busy} disabled={blocked || !selected || !draft.fields.reason.trim()} onClick={() => {
      try { const payload = projectTransition(basis, members, me, draft.fields.action, draft.fields.reason); setError(undefined); void command.submit('/projects/' + basis.project.id + '/transition', payload); }
      catch (cause) { setError(cause); }
    }}>确认{selected?.label ?? '项目阶段操作'}</Action>
  </View></Section>;
}

function ArchiveForm({ context, me, refill, refreshing, onReread, onCommitted }: {
  context: Context; me: Schemas['Me']; refill?: Refill; refreshing: boolean;
  onReread(notes: ArchiveNotes, files: FixedFile[]): Promise<void>; onCommitted(): void;
}) {
  const { basis, members } = context;
  const draft = useFormDraft(me, { kind: 'project-archive', id: basis.project.id, version: basis.project.version, hash: basis.fingerprint! }, 'archive');
  const [files, setFiles] = useState<FixedFile[]>(refill?.files ?? []), [business, setBusiness] = useState(false), [financial, setFinancial] = useState(false), [fresh, setFresh] = useState(false);
  const [error, setError] = useState<unknown>(), input = useRef<Schemas['ProjectArchiveInput']>(), proofScope = useRef<Scope>();
  const closeProof = () => proofScope.current?.close();
  useDidHide(closeProof); useEffect(() => closeProof, []);
  useEffect(() => { if (refill) draft.update({ description: refill.notes.summary, body: refill.notes.lessons, criteria: refill.notes.followUp, reason: refill.notes.reason }); }, []);
  const notes: ArchiveNotes = { summary: draft.fields.description ?? '', lessons: draft.fields.body ?? '', followUp: draft.fields.criteria ?? '', reason: draft.fields.reason };
  const command = useCommand<Schemas['ProjectArchiveReceipt']>(() => { draft.forget(); onCommitted(); }, draft.binding, async value => {
    if (!input.current || !archiveReceipt(value, basis, input.current, files, me.user.id)) return false;
    const scope = new Scope(); proofScope.current = scope;
    try {
      const current = currentCloseout(await client.request<Preview>('/projects/' + basis.project.id + '/closeout', { scope }), basis.project.id);
      const history = currentArchives(await client.request<Schemas['ProjectArchiveList']>('/projects/' + basis.project.id + '/archives?limit=10', { scope }), basis.project.id);
      return archiveProof(current, history, value, basis, input.current, files, me.user.id);
    } finally { scope.close(); }
  });
  const blocked = refreshing || command.busy || command.blocked;
  function update(change: Parameters<typeof draft.update>[0]) { draft.update(change); setBusiness(false); setFinancial(false); }
  return <Section title='复盘与明确归档'><View className='cp-pad'>
    <DraftNotice restored={draft.restored} /><Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} />
    {!draft.retained && <Text className='cp-notice'>文字超过本会话共享草稿预算，离开页面可能无法恢复；请妥善保留当前输入。</Text>}
    <TextField name='archive-summary' label='成果与交付总结' value={notes.summary} max={20000} multiline disabled={blocked} onChange={description => update({ description })} />
    <TextField name='archive-lessons' label='经验与复盘' value={notes.lessons} max={20000} multiline disabled={blocked} onChange={body => update({ body })} />
    <TextField name='archive-follow-up' label='后续跟进' value={notes.followUp} max={20000} multiline disabled={blocked} onChange={criteria => update({ criteria })} />
    <Text className='cp-muted cp-small'>无后续事项也请明确说明；文字声明不能代替真实未结事项的处置。</Text>
    <FixedFilePicker projectID={basis.project.id} value={files} disabled={blocked} optional purpose='复盘'
      onChange={value => { setFiles(value); setBusiness(false); setFinancial(false); }} />
    <TextField name='archive-reason' label='归档原因' value={notes.reason} max={200} multiline disabled={blocked} onChange={reason => update({ reason })} />
    <SelectField label='业务闭环核实' value={business ? 'yes' : 'no'} options={[[ 'no', '尚未确认' ], [ 'yes', '已核对任务、交付、风险、问题与变更' ]]} disabled={blocked} onChange={value => setBusiness(value === 'yes')} />
    <SelectField label='财务与义务核实' value={financial ? 'yes' : 'no'} options={[[ 'no', '尚未确认' ], [ 'yes', '已核对合同、费用、资金与订阅处置' ]]} disabled={blocked} onChange={value => setFinancial(value === 'yes')} />
    <MFAConfirmation onFresh={setFresh} disabled={blocked} />
    <Action loading={command.busy} disabled={blocked || !business || !financial || !fresh || !notes.summary.trim() || !notes.lessons.trim() || !notes.followUp.trim() || !notes.reason.trim()} onClick={() => {
      try { input.current = archivePayload(basis, members, me, notes, files, business, financial, fresh); setError(undefined); void command.submit('/projects/' + basis.project.id + '/archive', input.current); }
      catch (cause) { setError(cause); }
    }}>确认复盘并归档</Action>
    <Action secondary loading={refreshing} disabled={command.busy || refreshing} onClick={() => void onReread(notes, files)}>保留文字和选定版本，重新核查</Action>
    <Text className='cp-muted cp-small'>重新核查后两项声明和本人验证须重新完成；跨页面只恢复本会话有界文字，附件选择和验证不保存。结果未知时先核对实际项目及历史。</Text>
  </View></Section>;
}

function ArchiveHistory({ projectID }: { projectID: string }) {
  const [revision, setRevision] = useState(0), [records, setRecords] = useState<Schemas['ProjectArchive'][]>([]), [next, setNext] = useState<string | null>(null);
  const [visibility, setVisibility] = useState<string>(), [loading, setLoading] = useState(true), [error, setError] = useState<unknown>();
  const scope = useRef<Scope>(), pending = useRef(false), recordsRef = useRef<Schemas['ProjectArchive'][]>([]);
  function accept(value: unknown, append: boolean) {
    const result = currentArchives(value, projectID);
    if (append && result.items.some(item => recordsRef.current.some(record => record.id === item.id))) throw new MobileError('INVALID_RESPONSE', '归档分页出现重复记录，请重新读取。');
    const items = result.visibility === 'restricted' ? [] : append ? [...recordsRef.current, ...result.items] : result.items;
    recordsRef.current = items;
    setVisibility(result.visibility); setNext(result.next_cursor); setRecords(items);
  }
  useEffect(() => {
    const current = new Scope(); scope.current = current; pending.current = true; recordsRef.current = []; setLoading(true); setError(undefined); setRecords([]); setNext(null); setVisibility(undefined);
    void client.request<Schemas['ProjectArchiveList']>('/projects/' + projectID + '/archives?limit=10', { scope: current })
      .then(result => { if (!current.closed) accept(result, false); })
      .catch(cause => { if (!current.closed) { recordsRef.current = []; setError(cause); setRecords([]); setNext(null); setVisibility(undefined); } })
      .finally(() => { if (!current.closed) { setLoading(false); pending.current = false; } });
    return () => current.close();
  }, [projectID, revision]);
  async function more() {
    const current = scope.current; if (!current || current.closed || !next || pending.current) return;
    pending.current = true; setLoading(true); setError(undefined);
    try { const value = await client.request<Schemas['ProjectArchiveList']>('/projects/' + projectID + '/archives?limit=10&cursor=' + encodeURIComponent(next), { scope: current }); if (!current.closed) accept(value, true); }
    catch (cause) { if (!current.closed) { recordsRef.current = []; setError(cause); setRecords([]); setNext(null); setVisibility(undefined); } }
    finally { if (!current.closed) { pending.current = false; setLoading(false); } }
  }
  return <Section title='归档与复盘历史'>
    <Notice error={error} />{loading && <Busy />}
    {!error && (visibility === 'restricted' ? <Empty>权限受限，历史内容、数量和复盘尚未核实。</Empty>
      : records.map(record => <ArchiveRecord key={record.id} record={record} />))}
    {!loading && !error && visibility === 'complete' && !records.length && <Empty>当前核查范围内尚无归档记录。</Empty>}
    {visibility === 'complete' && !error && <Text className='cp-muted cp-small'>当前载入 {records.length} 份历史，后续按原服务分页读取。</Text>}
    {next && !error && <Action secondary loading={loading} onClick={() => void more()}>加载更早归档记录</Action>}
    <Action secondary disabled={loading} onClick={() => setRevision(old => old + 1)}>重新读取归档历史</Action>
  </Section>;
}
function ArchiveRecord({ record }: { record: Schemas['ProjectArchive'] }) {
  const [expanded, setExpanded] = useState(false);
  return <View className='cp-pad'>
    <Text className='cp-row-title'>{record.project_name} · 归档版本 {record.project_version}</Text>
    <Text className='cp-muted cp-small'>{overviewClock(record.created_at)} · 操作者当前姓名：{record.actor_name}</Text>
    <Text className='cp-small'>{record.current_basis_state === 'current' ? '当前仍与此归档依据一致。' : '当前依据已变化；以下保留归档当时事实。'}</Text>
    <Action secondary onClick={() => setExpanded(old => !old)}>{expanded ? '收起' : '查阅'}本次完整复盘</Action>
    {expanded && <>
      <Fact label='成果与交付总结'><Text className='cp-paragraph' selectable>{record.summary}</Text></Fact>
      <Fact label='经验与复盘'><Text className='cp-paragraph' selectable>{record.lessons_learned}</Text></Fact>
      <Fact label='后续跟进'><Text className='cp-paragraph' selectable>{record.follow_up}</Text></Fact>
      <Fact label='归档原因'><Text className='cp-paragraph'>{record.reason}</Text></Fact>
      <Text className='cp-muted cp-small'>当时已分别确认业务闭环及财务／合同／订阅处置；归档不产生费用或资金，不删除原记录。</Text>
      <FixedFiles files={record.attachments} /><Checklist groups={record.groups} projectID={record.project_id} historical />
    </>}
  </View>;
}
