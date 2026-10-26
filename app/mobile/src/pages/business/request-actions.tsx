import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { useCollection, useRecord } from '../../platform/collection';
import { useFormDraft } from '../../platform/form';
import { CommandNotice, DraftNotice, useCommand } from '../../platform/command';
import { go } from '../../platform/page';
import { MobileError, type Schemas } from '../../platform/session';
import { businessNames, businessRoutes, currentBusinessPayload, requestReceipt, type Business, type BusinessKind } from '../../platform/business-input';
import { Action, Busy, Check, Empty, Fact, Notice, Section, State, date } from '../../ui';
import { PayloadSnapshot } from '../detail/snapshot';
import { TextField } from './controls';

export function PrepareRequest({ kind, business, me, onCancel }: { kind: BusinessKind; business: Business; me: Schemas['Me']; onCancel(): void }) {
  const draft = useFormDraft(me, { kind: 'request-prepare', id: business.id, version: business.version }, 'prepare');
  const command = useCommand<Schemas['Request']>(request => { draft.forget(); go('request', request.id); }, draft.binding, result => requestReceipt(result, { kind, businessID: business.id, businessRevisionID: business.current_revision_id, state: 'draft' }));
  return <Section title='固定当前业务，建立申请草稿'><View className='cp-pad'><DraftNotice restored={draft.restored} /><Notice error={command.error} /><CommandNotice blocked={command.blocked} />
    <Text className='cp-paragraph'>{business.title} · 当前业务修订 {business.version}</Text><Text className='cp-muted cp-small'>此动作固定当前{nameOf(kind)}资料；尚不送审，也不分配审批步骤。</Text>
    <TextField name='prepare-reason' label='准备申请依据' value={draft.fields.reason} onChange={reason => draft.update({ reason })} max={200} multiline disabled={command.busy || command.blocked} />
    <Action disabled={command.blocked || !draft.fields.reason.trim() || !business.can_prepare_request} loading={command.busy} onClick={() => void command.submit('/requests', { kind, business_id: business.id, expected_business_version: business.version, business_revision_id: business.current_revision_id, business_content_hash: business.content_hash, reason: draft.fields.reason.trim() } satisfies Schemas['RequestCreate'])}>确认建立申请草稿</Action>
    <Action secondary disabled={command.busy} onClick={onCancel}>返回当前业务资料</Action>
  </View></Section>;
}
function nameOf(kind: BusinessKind) { return businessNames[kind]; }
export function RequestActions({ request, me, onCommitted }: { request: Schemas['Request']; me: Schemas['Me']; onCommitted(): void }) {
  const [mode, setMode] = useState<'none' | 'submit' | 'revise' | 'withdraw' | 'history'>('none');
  const supported = request.kind === 'contract' || request.kind === 'purchase' || request.kind === 'claim';
  return <Section title='申请操作与历史'><View className='cp-pad'>
    {mode === 'none' ? <>
      {supported && <Action secondary onClick={() => go(request.kind, request.business_id)}>查看当前业务资料</Action>}
      {supported && request.can_revise && <Action secondary onClick={() => setMode('revise')}>更新申请草稿</Action>}
      {request.can_submit && <Action onClick={() => setMode('submit')}>核对并送审</Action>}
      {request.can_withdraw && <Action secondary onClick={() => setMode('withdraw')}>撤回此申请</Action>}
      <Action secondary onClick={() => setMode('history')}>查看申请修订历史</Action>
    </> : <>
      {mode === 'submit' && request.can_submit && <SubmitRequest request={request} me={me} onCommitted={onCommitted} />}
      {mode === 'revise' && supported && request.can_revise && <ReviseRequest request={request} me={me} onCommitted={onCommitted} />}
      {mode === 'withdraw' && request.can_withdraw && <WithdrawRequest request={request} me={me} onCommitted={onCommitted} />}
      {mode === 'history' && <RequestHistory request={request} me={me} />}
      <Action secondary onClick={() => setMode('none')}>返回申请详情</Action>
    </>}
  </View></Section>;
}
function useReason(request: Schemas['Request'], me: Schemas['Me'], kind: 'request-submit' | 'request-revise' | 'request-withdraw') {
  return useFormDraft(me, { kind, id: request.id, version: request.version }, kind);
}
function Workflow({ workflow, historical = false }: { workflow: Schemas['WorkflowDefinition']; historical?: boolean }) {
  return <Section title={historical ? '该修订当时固定的审批顺序' : '本次将固定的审批顺序'}><View className='cp-pad'><Text className='cp-row-title'>{workflow.name} · 制度修订 {workflow.version}</Text>
    {workflow.steps.map(step => <Fact key={step.seq} label={step.seq + ' · ' + step.title}><Text>{step.assigned_user_name}</Text><Text className='cp-muted cp-small'>{step.requires_mfa ? '处理时需要两步验证' : '由指定人员登录处理'}</Text></Fact>)}
    <Text className='cp-muted cp-small cp-paragraph'>{workflow.self_approval_policy === 'allow' ? '此制度明确允许申请人审批本人申请。' : '此制度禁止本人审批。'}{workflow.withdrawal_allowed ? '审批期间允许申请人撤回。' : '审批期间不允许撤回。'}</Text>
  </View></Section>;
}
function SubmitRequest({ request, me, onCommitted }: { request: Schemas['Request']; me: Schemas['Me']; onCommitted(): void }) {
  const preview = useRecord<Schemas['SubmissionPreview']>('/requests/' + request.id + '/submission-preview');
  const draft = useReason(request, me, 'request-submit'), [checked, setChecked] = useState(false);
  const command = useCommand<Schemas['Request']>(() => { draft.forget(); onCommitted(); }, draft.binding, result => requestReceipt(result, { kind: request.kind, businessID: request.business_id, id: request.id, minimumVersion: request.version + 1, previousRevisionID: request.current_revision_id, businessRevisionID: request.payload.business_revision_id, state: 'in_review' }));
  const value = preview.value, matches = value && value.request_id === request.id && value.request_version === request.version && value.revision_id === request.current_revision_id && value.content_hash === request.content_hash;
  const stale = value && !matches ? new MobileError('PREVIEW_CHANGED', '申请预览与当前打开的修订不一致，请刷新申请后重新核对。') : undefined;
  return <><DraftNotice restored={draft.restored} /><Notice error={preview.error ?? stale ?? command.error} /><CommandNotice blocked={command.blocked} />
    {preview.loading ? <Busy /> : matches && <><Text className='cp-notice cp-paragraph'>以下内容与顺序将成为本次送审依据。确认送审后，普通业务编辑将停止。</Text>
      <PayloadSnapshot snapshot={value.payload} digest={value.content_hash} me={me} /><Workflow workflow={value.workflow} />
      <TextField name='submit-reason' label='送审依据' value={draft.fields.reason} onChange={reason => draft.update({ reason })} max={200} multiline disabled={command.busy || command.blocked} />
      <Check checked={checked} disabled={command.busy || command.blocked} onChange={setChecked}>我已核对以上固定内容、附件与审批顺序</Check>
      <Action disabled={!checked || !draft.fields.reason.trim() || command.blocked} loading={command.busy} onClick={() => void command.submit('/requests/' + request.id + '/submit', { expected_version: value.request_version, revision_id: value.revision_id, content_hash: value.content_hash, workflow_definition_id: value.workflow.id, preview_hash: value.preview_hash, reason: draft.fields.reason.trim() } satisfies Schemas['RequestSubmit'])}>确认以此内容送审</Action>
    </>}
  </>;
}
function ReviseRequest({ request, me, onCommitted }: { request: Schemas['Request']; me: Schemas['Me']; onCommitted(): void }) {
  const kind = request.kind as BusinessKind, business = useRecord<Business>(businessRoutes[kind] + '/' + request.business_id);
  const draft = useReason(request, me, 'request-revise'), [checked, setChecked] = useState(false);
  const command = useCommand<Schemas['Request']>(() => { draft.forget(); onCommitted(); }, draft.binding, result => requestReceipt(result, { kind: request.kind, businessID: request.business_id, id: request.id, minimumVersion: request.version + 1, previousRevisionID: request.current_revision_id, businessRevisionID: business.value?.current_revision_id, state: 'draft' })), value = business.value;
  return <><DraftNotice restored={draft.restored} /><Notice error={business.error ?? command.error} /><CommandNotice blocked={command.blocked} />
    {business.loading ? <Busy /> : value && <><Text className='cp-notice cp-paragraph'>请核对以下当前业务资料。确认后新增申请修订，旧内容与审批记录继续保留；本动作尚不重新送审。</Text>
      <PayloadSnapshot snapshot={currentBusinessPayload(kind, value)} digest={value.content_hash} me={me} heading='拟采用的当前业务修订' />
      <TextField name='revise-reason' label='更新申请依据' value={draft.fields.reason} onChange={reason => draft.update({ reason })} max={200} multiline disabled={command.busy || command.blocked} />
      <Check checked={checked} disabled={command.busy || command.blocked} onChange={setChecked}>我已核对本次将采用的完整业务修订</Check>
      <Action loading={command.busy} disabled={!checked || !draft.fields.reason.trim() || command.blocked} onClick={() => void command.submit('/requests/' + request.id + '/revisions', { expected_version: request.version, expected_business_version: value.version, business_revision_id: value.current_revision_id, business_content_hash: value.content_hash, reason: draft.fields.reason.trim() } satisfies Schemas['RequestRevise'])}>确认更新申请草稿</Action>
    </>}
  </>;
}
function WithdrawRequest({ request, me, onCommitted }: { request: Schemas['Request']; me: Schemas['Me']; onCommitted(): void }) {
  const draft = useReason(request, me, 'request-withdraw'), [checked, setChecked] = useState(false), command = useCommand<Schemas['Request']>(() => { draft.forget(); onCommitted(); }, draft.binding, result => requestReceipt(result, { kind: request.kind, businessID: request.business_id, id: request.id, minimumVersion: request.version + 1, revisionID: request.current_revision_id, state: 'withdrawn' }));
  return <><DraftNotice restored={draft.restored} /><Notice error={command.error} /><CommandNotice blocked={command.blocked} />
    <Text className='cp-paragraph'>撤回 {request.title} 的当前申请。已经执行的决定保留，尚未完成的步骤取消；这不会撤销已登记的履行或付款。</Text>
    <TextField name='withdraw-reason' label='撤回原因' value={draft.fields.reason} onChange={reason => draft.update({ reason })} max={200} multiline disabled={command.busy || command.blocked} />
    <Check checked={checked} disabled={command.busy || command.blocked} onChange={setChecked}>我确认撤回当前固定修订的申请</Check>
    <Action loading={command.busy} disabled={!checked || !draft.fields.reason.trim() || command.blocked} onClick={() => void command.submit('/requests/' + request.id + '/withdraw', { expected_version: request.version, revision_id: request.current_revision_id, content_hash: request.content_hash, reason: draft.fields.reason.trim() } satisfies Schemas['RequestWithdraw'])}>确认撤回此申请</Action>
  </>;
}
function RequestHistory({ request, me }: { request: Schemas['Request']; me: Schemas['Me'] }) {
  const history = useCollection<Schemas['RequestRevision']>('/requests/' + request.id + '/revisions?limit=10'), [selected, setSelected] = useState<string>();
  return <><Notice error={history.error} />{history.loading ? <Busy /> : history.items.length ? history.items.map(item => <View className='cp-editor-row' key={item.id}>
    <Action secondary onClick={() => setSelected(selected === item.id ? undefined : item.id)}>申请修订 {item.seq} · {item.payload.content.title}</Action>
    {selected === item.id && <><Text className='cp-muted cp-small cp-paragraph'>{date(item.created_at)} · {item.reason}</Text><Text className='cp-notice cp-paragraph'>以下是该修订当时固定的内容与决定，不代表当前申请状态。</Text>
      <PayloadSnapshot snapshot={item.payload} digest={item.content_hash} me={me} steps={item.steps} />{item.workflow && <Workflow workflow={item.workflow} historical />}
      {item.steps.map(step => <Fact key={step.id} label={step.seq + ' · ' + step.title}><Text>{step.assigned_user_name}</Text><State value={step.state} /><Text className='cp-paragraph'>{step.decision_reason}</Text></Fact>)}
    </>}
  </View>) : !history.error && <Empty>尚无可见申请修订。</Empty>}{history.next && <Action secondary loading={history.moreBusy} onClick={() => void history.more()}>加载更早申请修订</Action>}{Boolean(history.error) && <Action secondary onClick={history.reload}>重新读取申请历史</Action>}</>;
}
