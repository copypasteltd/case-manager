import { useState } from 'react';
import { concernStates, concernTransitions, makeConcernMutation, type ConcernTool, type ConcernValues, type Mutation } from '@copypaste/contracts/mutation';
import type { Schemas } from './api';
import { ErrorNotice, Field } from './components';

export function ConversationConcern({ tool, target, members, disabled, onChange }: {
  tool: ConcernTool;
  target: Schemas['Project'] | Schemas['Risk'] | Schemas['Issue'];
  members?: Schemas['ProjectMembers'];
  disabled: boolean;
  onChange(value: Mutation): void;
}) {
  const creating = tool.startsWith('create_'), risk = tool.endsWith('_risk'), existing = creating ? undefined : target as Schemas['Risk'] & Schemas['Issue'];
  const [title, setTitle] = useState(existing?.title ?? ''), [description, setDescription] = useState(existing?.description ?? '');
  const [owner, setOwner] = useState(existing?.owner_id ?? ''), [severity, setSeverity] = useState<string>(existing?.severity ?? '');
  const [due, setDue] = useState(existing?.due_at ?? ''), [dueMode, setDueMode] = useState(existing?.due_at ? 'set' : 'clear');
  const [probability, setProbability] = useState<string>(existing?.probability ?? ''), [mitigation, setMitigation] = useState(existing?.mitigation ?? '');
  const [resolution, setResolution] = useState(existing?.resolution ?? ''), [state, setState] = useState(''), [reason, setReason] = useState('');
  const [acknowledged, setAcknowledged] = useState(false), [error, setError] = useState<unknown>();
  const activeMembers = members?.items.filter(m => m.status === 'active') ?? [];
  const states = creating ? [] : concernTransitions[risk ? 'risk' : 'issue'][target.state] ?? [];
  function prepare() {
    try {
      const values: ConcernValues = { title, description, owner_id: owner, severity, due_at: dueMode === 'clear' ? null : due };
      if (risk) { values.probability = probability || null; values.mitigation = mitigation; } else values.resolution = resolution;
      if (state) values.state = state;
      onChange(makeConcernMutation(tool, target, values, reason));
    } catch (cause) { setError(cause); }
  }
  return <div className="conversation-concern-editor">
    <ErrorNotice error={error} />
    <Field label="事项标题"><input value={title} maxLength={400} disabled={disabled} onChange={e => setTitle(e.target.value)} /></Field>
    <Field label="完整说明"><textarea rows={4} value={description} maxLength={40000} disabled={disabled} onChange={e => setDescription(e.target.value)} /></Field>
    <Field label="事项负责人"><select value={owner} disabled={disabled} onChange={e => setOwner(e.target.value)}>
      <option value="">请明确选择项目成员</option>
      {owner && !activeMembers.some(m => m.id === owner) && <option value={owner}>当前负责人（待核对成员）</option>}
      {activeMembers.map(m => <option key={m.id} value={m.id}>{m.display_name}</option>)}
    </select></Field>
    <Field label="严重程度"><select value={severity} disabled={disabled} onChange={e => setSeverity(e.target.value)}><option value="">请明确选择严重程度</option><option value="low">低</option><option value="medium">中</option><option value="high">高</option><option value="critical">严重</option></select></Field>
    <Field label="截止时刻处理"><select value={dueMode} disabled={disabled} onChange={e => setDueMode(e.target.value)}><option value="clear">明确不设置／清空</option><option value="set">设置准确时刻</option></select></Field>
    {dueMode === 'set' && <Field label="事项截止时刻"><input value={due} disabled={disabled} onChange={e => setDue(e.target.value)} placeholder="2027-03-05T18:00:00.123456+08:00" /></Field>}
    {risk ? <>
      <Field label="发生概率"><select value={probability} disabled={disabled} onChange={e => setProbability(e.target.value)}><option value="">明确未评估／清空</option><option value="low">低</option><option value="medium">中</option><option value="high">高</option></select></Field>
      <Field label="缓解措施"><textarea rows={4} value={mitigation} maxLength={40000} disabled={disabled} onChange={e => setMitigation(e.target.value)} /></Field>
    </> : <Field label="解决说明"><textarea rows={4} value={resolution} maxLength={40000} disabled={disabled} onChange={e => setResolution(e.target.value)} /></Field>}
    {!creating && <Field label="事项状态处理"><select value={state} disabled={disabled} onChange={e => { setState(e.target.value); setAcknowledged(false); }}><option value="">保持当前状态：{concernStates[target.state] ?? target.state}</option>{states.map(s => <option key={s} value={s}>{concernStates[s]}</option>)}</select></Field>}
    {state === 'realized' && <label className="form-note"><input type="checkbox" checked={acknowledged} disabled={disabled} onChange={e => setAcknowledged(e.target.checked)} />我已核对：本次风险发生会新增一个关联问题</label>}
    <Field label="本次更新原因"><input value={reason} maxLength={400} disabled={disabled} onChange={e => setReason(e.target.value)} /></Field>
    <p className="form-note">请完整核对全部资料。状态保持时不发送状态字段；截止时刻和概率为空时明确发送清空。创建只建立待处理事项。风险发生将调用原业务创建一个关联问题，需要同时具备问题创建权限。</p>
    <button type="button" className="button secondary" disabled={disabled || !activeMembers.some(m => m.id === owner) || state === 'realized' && !acknowledged} onClick={prepare}>将这项明确更新加入请求</button>
  </div>;
}
