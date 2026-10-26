import { Text, View } from '@tarojs/components';
import { Picker, Textarea } from '../../platform/components';
import { useState } from 'react';
import { type Schemas } from '../../platform/session';
import { useFormDraft } from '../../platform/form';
import { CommandNotice, DraftNotice, MFAConfirmation, useCommand } from '../../platform/command';
import { Action, Field, Notice, Section } from '../../ui';

export function TaskActions({ task, me, onCommitted }: { task: Schemas['Task']; me: Schemas['Me']; onCommitted(): void }) {
  const actions: [string, string][] = [];
  if (me.actions.includes('task.transition')) {
    if (task.state === 'todo') actions.push(['start', '开始处理']);
    if (task.state === 'in_progress') actions.push(task.review_required ? ['review', '提交审核'] : ['complete', '标记完成']);
    if (task.state === 'in_review') actions.push(['complete', '确认完成'], ['return', '退回处理']);
    if (task.state === 'blocked') actions.push(['resume', '恢复推进']);
    if (!['done', 'cancelled', 'blocked'].includes(task.state)) actions.push(['block', '标记受阻']);
    if (!['done', 'cancelled'].includes(task.state)) actions.push(['cancel', '取消任务']);
  }
  if (['done', 'cancelled'].includes(task.state) && me.actions.includes('task.reopen')) actions.push(['reopen', '重新打开']);
  const draft = useFormDraft(me, { kind: 'task', id: task.id, version: task.version }, actions[0]?.[0] ?? '');
  const command = useCommand(() => { draft.forget(); onCommitted(); }, draft.binding);
  const choice = Math.max(0, actions.findIndex(item => item[0] === draft.fields.action)), reason = draft.fields.reason;
  if (!actions.length) return null;
  return <Section title='推进任务'><View className='cp-pad'><DraftNotice restored={draft.restored} /><Notice error={command.error} /><CommandNotice blocked={command.blocked} /><Field label='本次动作'><Picker ariaLabel='本次动作' mode='selector' range={actions.map(item => item[1])} value={choice} disabled={command.busy || command.blocked} onChange={event => { const action = actions[Number(event.detail.value)]; if (action) draft.update({ action: action[0] }); }}><View className='cp-picker'>{actions[choice]?.[1] ?? '请选择'}</View></Picker></Field><Field label='变更原因'><Textarea ariaLabel='变更或决定原因' className='cp-textarea' value={reason} maxlength={200} disabled={command.busy || command.blocked} onInput={event => draft.update({ reason: event.detail.value })} /></Field><Action disabled={!reason.trim() || command.blocked || !actions[choice]} loading={command.busy} onClick={() => void command.submit('/tasks/' + task.id + '/transition', { expected_version: task.version, action: actions[choice][0], reason: reason.trim() })}>确认{actions[choice]?.[1]}</Action></View></Section>;
}

export function DecisionForm({ request, step, me, onCommitted }: { request: Schemas['Request']; step: Schemas['ApprovalTask']; me: Schemas['Me']; onCommitted(): void }) {
  const choices = [['approve', '同意当前修订'], ['return', '退回补充'], ['reject', '拒绝申请']] as const;
  const draft = useFormDraft(me, { kind: 'approval', id: request.id, version: request.version, revision: request.current_revision_id, hash: request.content_hash, step: step.id, stepVersion: step.version }, 'approve');
  const [fresh, setFresh] = useState(false), choice = Math.max(0, choices.findIndex(item => item[0] === draft.fields.action)), reason = draft.fields.reason;
  const command = useCommand(() => { draft.forget(); onCommitted(); }, draft.binding), decision = choices[choice][0];
  return <View><Text className='cp-muted cp-small cp-paragraph'>本次决定针对以上固定申请内容。请先核对条款、金额、收付计划和附件依据。</Text><DraftNotice restored={draft.restored} /><Notice error={command.error} /><CommandNotice blocked={command.blocked} /><Field label='本次决定'><Picker ariaLabel='本次决定' mode='selector' range={choices.map(item => item[1])} value={choice} disabled={command.busy || command.blocked} onChange={event => { const action = choices[Number(event.detail.value)]; if (action) draft.update({ action: action[0] }); }}><View className='cp-picker'>{choices[choice][1]}</View></Picker></Field><Field label={decision === 'approve' ? '核对说明（可选）' : '决定原因（必填）'}><Textarea ariaLabel='变更或决定原因' className='cp-textarea' value={reason} maxlength={20000} disabled={command.busy || command.blocked} onInput={event => draft.update({ reason: event.detail.value })} /></Field>{step.requires_mfa && <MFAConfirmation onFresh={setFresh} disabled={command.busy || command.blocked} />}<Action loading={command.busy} disabled={command.blocked || (decision !== 'approve' && !reason.trim()) || (step.requires_mfa && !fresh)} onClick={() => void command.submit('/approval-steps/' + step.id + '/decisions', { expected_version: step.version, request_version: request.version, revision_id: request.current_revision_id, content_hash: request.content_hash, decision, reason: reason.trim() } satisfies Schemas['ApprovalDecision'])}>确认{choices[choice][1]}</Action></View>;
}
