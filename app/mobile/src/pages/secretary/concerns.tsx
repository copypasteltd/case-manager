import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { concernStates, concernTransitions, makeConcernMutation, type ConcernTool, type ConcernValues, type Mutation } from '@copypaste/contracts/mutation';
import { MobileError, type Schemas } from '../../platform/session';
import { Action, Check, Notice } from '../../ui';
import { SelectField, TextField } from '../business/controls';

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
  const [acknowledged, setAcknowledged] = useState(false), [error, setError] = useState<MobileError>();
  const activeMembers = members?.items.filter(m => m.status === 'active') ?? [];
  const states = creating ? [] : concernTransitions[risk ? 'risk' : 'issue'][target.state] ?? [];
  function prepare() {
    try {
      const values: ConcernValues = { title, description, owner_id: owner, severity, due_at: dueMode === 'clear' ? null : due };
      if (risk) { values.probability = probability || null; values.mitigation = mitigation; } else values.resolution = resolution;
      if (state) values.state = state;
      onChange(makeConcernMutation(tool, target, values, reason));
    } catch (cause) { setError(new MobileError('INPUT_REQUIRED', cause instanceof Error ? cause.message : '请完整核对事项资料。')); }
  }
  return <View>
    <Notice error={error} />
    <TextField name='concern-title' label='事项标题' value={title} max={400} disabled={disabled} onChange={setTitle} />
    <TextField name='concern-description' label='完整说明' multiline value={description} max={40000} disabled={disabled} onChange={setDescription} />
    <SelectField label='事项负责人' value={owner} disabled={disabled} onChange={setOwner} options={[
      ['', '请明确选择项目成员'], ...(owner && !activeMembers.some(m => m.id === owner) ? [[owner, '当前负责人（待核对成员）'] as const] : []),
      ...activeMembers.map(m => [m.id, m.display_name] as const),
    ]} />
    <SelectField label='严重程度' value={severity} disabled={disabled} onChange={setSeverity} options={[[ '', '请明确选择严重程度' ], ['low', '低'], ['medium', '中'], ['high', '高'], ['critical', '严重']]} />
    <SelectField label='截止时刻处理' value={dueMode} disabled={disabled} onChange={setDueMode} options={[[ 'clear', '明确不设置／清空' ], ['set', '设置准确时刻']]} />
    {dueMode === 'set' && <TextField name='concern-due' label='事项截止时刻' value={due} disabled={disabled} onChange={setDue} hint='例如2027-03-05T18:00:00.123456+08:00，含时区，最多六位小数秒' />}
    {risk ? <>
      <SelectField label='发生概率' value={probability} disabled={disabled} onChange={setProbability} options={[[ '', '明确未评估／清空' ], ['low', '低'], ['medium', '中'], ['high', '高']]} />
      <TextField name='concern-mitigation' label='缓解措施' multiline value={mitigation} max={40000} disabled={disabled} onChange={setMitigation} />
    </> : <TextField name='concern-resolution' label='解决说明' multiline value={resolution} max={40000} disabled={disabled} onChange={setResolution} />}
    {!creating && <SelectField label='事项状态处理' value={state} disabled={disabled} onChange={s => { setState(s); setAcknowledged(false); }} options={[
      ['', '保持当前状态：' + (concernStates[target.state] ?? target.state)], ...states.map(s => [s, concernStates[s]] as const),
    ]} />}
    {state === 'realized' && <Check checked={acknowledged} disabled={disabled} onChange={setAcknowledged}>我已核对：本次风险发生会新增一个关联问题</Check>}
    <TextField name='concern-reason' label='本次更新原因' value={reason} max={400} disabled={disabled} onChange={setReason} />
    <Text className='cp-small cp-paragraph'>请完整核对全部资料。状态保持时不发送状态字段；截止时刻和概率为空时明确发送清空。创建只建立待处理事项。风险发生将新增一个关联问题，需要同时具备问题创建权限。</Text>
    <Action secondary disabled={disabled || !activeMembers.some(m => m.id === owner) || state === 'realized' && !acknowledged} onClick={prepare}>将这项明确更新加入请求</Action>
  </View>;
}
