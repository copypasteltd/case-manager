import { Text, View } from '@tarojs/components';
import Taro, { useRouter } from '@tarojs/taro';
import { useState } from 'react';
import { Picker, Input, Textarea } from '../../platform/components';
import { useCollection, useRecord } from '../../platform/collection';
import { CommandNotice, DraftNotice, MFAConfirmation, useCommand } from '../../platform/command';
import { FixedFilePicker, FixedFiles, type FixedFile } from '../../platform/fixed-files';
import { useFormDraft } from '../../platform/form';
import { identifier, projectSection, usePage } from '../../platform/page';
import type { Schemas } from '../../platform/session';
import { client } from '../../platform/taro';
import { Action, Busy, Check, Empty, Fact, Field, Notice, Page, Row, Section, State, date } from '../../ui';

type Head = Schemas['Deliverable'];
type Revision = Schemas['DeliverableRevision'];
type Value = { project: Schemas['Project']; head?: Head; revision?: Revision; members: Schemas['ProjectMembers'] };
type Mode = 'view' | 'edit' | 'submit' | 'decide';

export default function DeliverablePage() {
  const params = useRouter().params, [mode, setMode] = useState<Mode>('view');
  const page = usePage<Value>(async scope => {
    const head = params.id ? await client.request<Head>('/deliverables/' + identifier(params.id), { scope }) : undefined;
    const projectID = head?.project_id ?? identifier(params.project_id);
    const project = await client.request<Schemas['Project']>('/projects/' + projectID, { scope });
    const members = await client.request<Schemas['ProjectMembers']>('/projects/' + projectID + '/members', { scope });
    const revisionID = params.revision ? identifier(params.revision) : head?.current_revision_id;
    const revision = head && revisionID ? await client.request<Revision>('/deliverables/' + head.id + '/revisions/' + revisionID, { scope }) : undefined;
    return { project, head, revision, members };
  });
  const value = page.result?.value, historical = Boolean(params.revision);
  function committed() { setMode('view'); void page.reload(); }
  const canCreate = value && !value.head && page.result!.me.actions.includes('deliverable.create') && !['closing', 'archived', 'cancelled'].includes(value.project.state) && value.members.memberships?.find(item => item.user_id === page.result!.me.user.id)?.project_role !== 'viewer';
  const title = value ? historical ? '交付修订历史' : value.head?.name ?? '新建项目交付' : '项目交付';
  return <Page title={title} home><Notice error={page.error} />{page.loading ? <Busy /> : value && <>
    <View className='cp-choice'><Text className='cp-muted cp-small'>{value.project.name}</Text>{value.head && <>{historical && <Text className='cp-muted cp-small'>当前交付阶段：</Text>}<State value={value.head.state} /></>}</View>
    {!value.head ? canCreate ? <DeliverableEditor key={value.project.id + ':' + value.project.version} value={value} me={page.result!.me} onCommitted={committed} /> : <Empty>当前权限或项目阶段不允许新建交付。</Empty> : <>
      {value.head.protocol_version !== 1 && <View className='cp-notice'><Text>这是来源待核实的旧交付记录。当前页面仅供查阅，不将旧记录视为已按现行流程验收。</Text></View>}
      {!historical && <Section title='当前交付资料'><Fact label='交付名称'><Text>{value.head.name}</Text></Fact><Fact label='验收准则'><Text className='cp-paragraph'>{value.head.acceptance_criteria}</Text></Fact><Fact label='负责人'><Text>{memberName(value.members, value.head.owner_id)}</Text></Fact><Fact label='当前修订'><Text>{value.head.version} · {date(value.head.updated_at)}</Text></Fact>{value.head.milestone_id && <View className='cp-pad'><Action secondary onClick={() => void Taro.navigateTo({ url: '/pages/detail/index?kind=milestone&id=' + value.head!.milestone_id })}>查看关联里程碑</Action></View>}</Section>}
      {value.revision && <RevisionSummary revision={value.revision} members={value.members} historical={historical} />}
      {!historical && mode === 'view' && <>
        {value.head.can_edit && <Action secondary onClick={() => setMode('edit')}>编辑交付资料</Action>}
        {value.head.can_submit && <Action onClick={() => setMode('submit')}>{value.head.state === 'accepted' ? '提交新的交付修订' : '提交固定修订'}</Action>}
        {value.head.can_decide && value.revision && <Action onClick={() => setMode('decide')}>核实并登记验收</Action>}
      </>}
      {!historical && mode !== 'view' && <>
        {mode === 'edit' && value.head.can_edit && <DeliverableEditor key={value.head.id + ':' + value.head.version + ':edit'} value={value} me={page.result!.me} onCommitted={committed} />}
        {mode === 'submit' && value.head.can_submit && <Submission key={value.head.id + ':' + value.head.version + ':submit'} head={value.head} me={page.result!.me} onCommitted={committed} />}
        {mode === 'decide' && value.head.can_decide && value.revision && <Decision key={value.head.id + ':' + value.head.version + ':' + value.revision.id} head={value.head} revision={value.revision} me={page.result!.me} onCommitted={committed} />}
        <Action secondary onClick={() => setMode('view')}>返回交付详情</Action>
      </>}
      <RevisionHistory id={value.head.id} />
    </>}
    <Action secondary onClick={() => projectSection(value.project.id, 'deliverables')}>返回项目交付列表</Action>
  </>}
    <Action secondary loading={page.loading} onClick={() => void page.reload()}>刷新当前交付</Action>
  </Page>;
}

function memberName(members: Schemas['ProjectMembers'], id: string) { return members.items.find(user => user.id === id)?.display_name ?? '当前姓名不可查阅'; }

function RevisionSummary({ revision, members, historical }: { revision: Revision; members: Schemas['ProjectMembers']; historical: boolean }) {
  const decision = revision.decision;
  return <Section title={'第' + revision.seq + '次提交 · ' + (historical ? '历史固定依据' : '本次固定依据')}>
    <Fact label='当时的交付名称'><Text>{revision.name}</Text></Fact>
    <Fact label='当时的验收准则'><Text className='cp-paragraph'>{revision.acceptance_criteria}</Text></Fact>
    <Fact label='交付说明'><Text className='cp-paragraph'>{revision.description}</Text></Fact>
    <Fact label='提交人（当前名称）'><Text>{memberName(members, revision.submitted_by)} · {date(revision.submitted_at)}</Text></Fact>
    <Fact label='固定内容摘要'><Text className='cp-code'>{revision.content_hash}</Text></Fact>
    <View className='cp-pad'><FixedFiles files={revision.files} /></View>
    <Fact label='这一修订的验收记录'>{decision ? <>
      <Text className='cp-row-title'>{decision.decision === 'accept' ? '当时已验收' : '当时已退回'}</Text>
      <Text className='cp-paragraph'>{decision.reason}</Text>
      <Text className='cp-muted cp-small'>{memberName(members, decision.decided_by)} · {date(decision.decided_at)}</Text>
      <Text className='cp-muted cp-small'>文件核实：{decision.files_verified ? '已声明核实' : '未声明核实'}；准则核实：{decision.criteria_verified ? '已声明核实' : '未声明核实'}</Text>
    </> : <Text>这一修订尚无人工验收记录。</Text>}</Fact>
    {historical && <View className='cp-notice'><Text>以上是提交时固定的内容及当时决定，不代表当前修订已通过验收。</Text></View>}
  </Section>;
}

function RevisionHistory({ id }: { id: string }) {
  const history = useCollection<Revision>('/deliverables/' + id + '/revisions?limit=20');
  return <Section title='提交与验收历史'><Notice error={history.error} />{history.loading ? <Busy /> : history.items.length ? history.items.map(revision => <Row key={revision.id} title={'第' + revision.seq + '次提交 · ' + revision.name} subtitle={date(revision.submitted_at) + ' · ' + (revision.decision ? revision.decision.decision === 'accept' ? '当时已验收' : '当时已退回' : '尚无验收记录')} onClick={() => void Taro.navigateTo({ url: '/pages/deliverable/index?id=' + id + '&revision=' + revision.id })} />) : !history.error && <Empty>尚未提交固定修订。</Empty>}
    {Boolean(history.error) && <View className='cp-pad'><Action secondary onClick={history.reload}>重新读取交付历史</Action></View>}
    {history.next && <View className='cp-pad'><Action secondary loading={history.moreBusy} onClick={() => void history.more()}>加载更多交付历史</Action></View>}
  </Section>;
}

function DeliverableEditor({ value, me, onCommitted }: { value: Value; me: Schemas['Me']; onCommitted(): void }) {
  const { project, head, members } = value;
  const draft = useFormDraft(me, { kind: head ? 'deliverable-edit' : 'deliverable-create', id: head?.id ?? project.id, version: head?.version ?? project.version }, 'save', { name: head?.name ?? '', criteria: head?.acceptance_criteria ?? '', owner: head?.owner_id ?? '', milestone: head?.milestone_id ?? '' });
  const command = useCommand<Schemas['DeliverableReceipt']>(receipt => { draft.forget(); if (!head) void Taro.redirectTo({ url: '/pages/deliverable/index?id=' + receipt.id }); else onCommitted(); }, draft.binding);
  const milestones = useCollection<Schemas['Milestone']>('/projects/' + project.id + '/milestones?limit=20');
  const selectedMilestone = useRecord<Schemas['Milestone']>(draft.fields.milestone ? '/milestones/' + identifier(draft.fields.milestone) : null);
  const users = members.items.filter(user => user.status === 'active');
  const ownerIndex = users.findIndex(user => user.id === draft.fields.owner) + 1;
  const options = milestones.items.filter(item => item.state === 'open').map(item => ({ id: item.id, title: item.title }));
  if (selectedMilestone.value?.id === draft.fields.milestone && selectedMilestone.value?.state === 'open' && selectedMilestone.value.project_id === project.id && !options.some(item => item.id === selectedMilestone.value!.id)) options.unshift({ id: selectedMilestone.value.id, title: selectedMilestone.value.title });
  const milestoneIndex = options.findIndex(item => item.id === draft.fields.milestone) + 1;
  const name = draft.fields.name ?? '', criteria = draft.fields.criteria ?? '', reason = draft.fields.reason;
  const ready = !!name.trim() && !!criteria.trim() && !!reason.trim() && ownerIndex > 0 && (!draft.fields.milestone || milestoneIndex > 0);
  const disabled = command.busy || command.blocked;
  return <Section title={head ? '编辑交付资料' : '新建项目交付'}><View className='cp-pad'>
    <DraftNotice restored={draft.restored} /><Notice error={command.error} /><CommandNotice blocked={command.blocked} />
    <Field label='交付名称'><Input ariaLabel='交付名称' className='cp-input' value={name} maxlength={200} disabled={disabled} onInput={event => draft.update({ name: event.detail.value })} /></Field>
    <Field label='验收准则'><Textarea ariaLabel='验收准则' className='cp-textarea' value={criteria} maxlength={20000} disabled={disabled} onInput={event => draft.update({ criteria: event.detail.value })} /></Field>
    <Field label='交付负责人'><Picker ariaLabel='交付负责人' mode='selector' range={['请选择负责人', ...users.map(user => user.display_name)]} value={ownerIndex} disabled={disabled} onChange={event => draft.update({ owner: users[Number(event.detail.value) - 1]?.id ?? '' })}><View className='cp-picker'>{users[ownerIndex - 1]?.display_name ?? '请选择负责人'}</View></Picker></Field>
    <Field label='关联里程碑（可选）'><Picker ariaLabel='关联里程碑' mode='selector' range={['不关联里程碑', ...options.map(item => item.title)]} value={milestoneIndex} disabled={disabled} onChange={event => draft.update({ milestone: options[Number(event.detail.value) - 1]?.id ?? '' })}><View className='cp-picker'>{options[milestoneIndex - 1]?.title ?? (draft.fields.milestone ? selectedMilestone.loading ? '正在核对已选里程碑…' : '已选里程碑不可用，请重新选择' : '不关联里程碑')}</View></Picker></Field>
    <Notice error={selectedMilestone.error} />
    <Notice error={milestones.error} />{Boolean(milestones.error) && <Action secondary onClick={milestones.reload}>重新读取里程碑候选</Action>}{milestones.next && <Action secondary loading={milestones.moreBusy} onClick={() => void milestones.more()}>加载更多里程碑候选</Action>}
    <Field label='保存原因'><Textarea ariaLabel='保存交付原因' className='cp-textarea' value={reason} maxlength={200} disabled={disabled} onInput={event => draft.update({ reason: event.detail.value })} /></Field>
    <Action loading={command.busy} disabled={disabled || !ready} onClick={() => {
      const body: Schemas['DeliverableCreate'] = { name: name.trim(), acceptance_criteria: criteria.trim(), owner_id: draft.fields.owner!, milestone_id: draft.fields.milestone || null, reason: reason.trim() };
      void command.submit(head ? '/deliverables/' + head.id : '/projects/' + project.id + '/deliverables', head ? { ...body, expected_version: head.version } satisfies Schemas['DeliverablePatch'] : body, head ? 'PATCH' : 'POST');
    }}>{head ? '保存交付资料' : '创建项目交付'}</Action>
  </View></Section>;
}

function Submission({ head, me, onCommitted }: { head: Head; me: Schemas['Me']; onCommitted(): void }) {
  const draft = useFormDraft(me, { kind: 'deliverable-submit', id: head.id, version: head.version }, 'submit');
  const [files, setFiles] = useState<FixedFile[]>([]);
  const command = useCommand(() => { draft.forget(); onCommitted(); }, draft.binding), description = draft.fields.description ?? '', reason = draft.fields.reason;
  const disabled = command.busy || command.blocked;
  return <Section title='提交固定交付修订'><View className='cp-pad'><DraftNotice restored={draft.restored} /><Notice error={command.error} /><CommandNotice blocked={command.blocked} />
    <Text className='cp-paragraph'>本次会固定当前名称、验收准则、负责人、里程碑和所选文件版本。文件更新不会替换这次提交的依据；已验收交付重新提交后，需要独立验收新修订。</Text>
    <FixedFilePicker projectID={head.project_id} value={files} onChange={setFiles} disabled={disabled} />
    <Field label='本次交付说明'><Textarea ariaLabel='本次交付说明' className='cp-textarea' value={description} maxlength={20000} disabled={disabled} onInput={event => draft.update({ description: event.detail.value })} /></Field>
    <Field label='提交原因'><Textarea ariaLabel='提交交付原因' className='cp-textarea' value={reason} maxlength={200} disabled={disabled} onInput={event => draft.update({ reason: event.detail.value })} /></Field>
    <Action loading={command.busy} disabled={disabled || !files.length || !description.trim() || !reason.trim()} onClick={() => void command.submit('/deliverables/' + head.id + '/submit', { expected_version: head.version, expected_revision_id: head.current_revision_id, file_version_ids: files.map(file => file.version_id), description: description.trim(), reason: reason.trim() } satisfies Schemas['DeliverableSubmit'])}>确认提交固定修订</Action>
  </View></Section>;
}

function Decision({ head, revision, me, onCommitted }: { head: Head; revision: Revision; me: Schemas['Me']; onCommitted(): void }) {
  const draft = useFormDraft(me, { kind: 'deliverable-decision', id: head.id, version: head.version, revision: revision.id, hash: revision.content_hash }, 'request_changes');
  const [filesVerified, setFilesVerified] = useState(false), [criteriaVerified, setCriteriaVerified] = useState(false), [fresh, setFresh] = useState(false);
  const command = useCommand(() => { draft.forget(); onCommitted(); }, draft.binding);
  const accepting = draft.fields.action === 'accept', self = revision.submitted_by === me.user.id, reason = draft.fields.reason;
  const disabled = command.busy || command.blocked;
  return <Section title='核实并登记交付验收'><View className='cp-pad'><DraftNotice restored={draft.restored} /><Notice error={command.error} /><CommandNotice blocked={command.blocked} />
    <Text className='cp-paragraph'>决定仅针对第{revision.seq}次提交及以上固定内容。文件元数据不等于文件内容核实；请通过可用渠道实际查阅文件，再作出声明。</Text>
    {self && <View className='cp-notice'><Text>你是这一修订的提交人，可以退回补充，不能自验通过。通过验收需要另一名获授权人员。</Text></View>}
    <Field label='验收决定'><Picker ariaLabel='验收决定' mode='selector' range={['退回补充', '通过验收']} value={accepting ? 1 : 0} disabled={disabled} onChange={event => { draft.update({ action: Number(event.detail.value) === 1 ? 'accept' : 'request_changes' }); setFilesVerified(false); setCriteriaVerified(false); }}><View className='cp-picker'>{accepting ? '通过验收' : '退回补充'}</View></Picker></Field>
    {accepting && <><Check checked={filesVerified} disabled={disabled || self} onChange={setFilesVerified}>我已实际核实这一修订的全部固定文件版本。</Check><Check checked={criteriaVerified} disabled={disabled || self} onChange={setCriteriaVerified}>我已逐项核实这些文件满足本次固定验收准则。</Check></>}
    <Field label='验收说明或退回原因'><Textarea ariaLabel='验收说明或退回原因' className='cp-textarea' value={reason} maxlength={20000} disabled={disabled} onInput={event => draft.update({ reason: event.detail.value })} /></Field>
    <MFAConfirmation onFresh={setFresh} disabled={disabled} />
    <Action loading={command.busy} disabled={disabled || !reason.trim() || !fresh || (accepting && (self || !filesVerified || !criteriaVerified))} onClick={() => void command.submit('/deliverables/' + head.id + '/decisions', { expected_version: head.version, revision_id: revision.id, content_hash: revision.content_hash, decision: accepting ? 'accept' : 'request_changes', files_verified: filesVerified, criteria_verified: criteriaVerified, reason: reason.trim() } satisfies Schemas['DeliverableDecisionInput'])}>{accepting ? '确认通过这一修订' : '确认退回这一修订'}</Action>
  </View></Section>;
}
