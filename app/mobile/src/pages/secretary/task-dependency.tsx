import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { makeTaskDependency, mutationValue, type Mutation } from '@copypaste/contracts/mutation';
import { useRecord } from '../../platform/collection';
import { MobileError, type Schemas } from '../../platform/session';
import { Action, Busy, Fact, Notice } from '../../ui';
import { ChoiceList, SelectField, TextField } from '../business/controls';

export function ConversationTaskDependency({ task, disabled, onChange }: { task: Schemas['Task']; disabled: boolean; onChange(value: Mutation): void }) {
  const [selected, setSelected] = useState(''), [picking, setPicking] = useState(false), [action, setAction] = useState(''), [reason, setReason] = useState(''), [error, setError] = useState<MobileError>();
  const predecessor = useRecord<Schemas['Task']>(selected ? '/tasks/' + selected : null);
  const edges = useRecord<Schemas['TaskDependencies']>('/tasks/' + task.id + '/dependencies');
  const current = predecessor.error ? undefined : predecessor.value;
  if (picking) return <ChoiceList<Schemas['Task']> title='选择前置任务' path={'/projects/' + task.project_id + '/tasks?limit=20'} selected={selected} label={t => t.title + ' · ' + mutationValue(t.state, 'state')} eligible={t => t.id !== task.id} onSelect={t => { if (!disabled) { setSelected(t.id); setPicking(false); setError(undefined); } }} onClose={() => setPicking(false)} />;
  return <View>
    <Notice error={error ?? predecessor.error ?? edges.error} />
    <Text className='cp-small cp-paragraph'>当前选中的任务是后续任务。新增依赖后，须先完成前置任务才能开始后续任务。每次仅调整一条关系。</Text>
    <Action secondary disabled={disabled} onClick={() => setPicking(true)}>{selected ? '更换前置任务' : '选择前置任务'}</Action>
    {(predecessor.loading || edges.loading) && <Busy />}
    {current && !predecessor.loading && <Fact label='前置任务'><Text>{current.title} · 基础版本 {current.version} · {mutationValue(current.state, 'state')}</Text></Fact>}
    {current && edges.value && <Fact label='当前关系'><Text>{edges.value.items.some(item => item.predecessor_id === current.id) ? '存在依赖' : '没有依赖'}</Text></Fact>}
    <SelectField label='依赖动作' value={action} options={[[ '', '请明确选择' ], [ 'add', '新增依赖' ], [ 'remove', '解除依赖' ]]} onChange={value => { setAction(value); setError(undefined); }} disabled={disabled} />
    <TextField name='dependency-reason' label='本次依赖更新原因' value={reason} onChange={value => { setReason(value); setError(undefined); }} disabled={disabled} max={400} />
    <Text className='cp-small cp-paragraph'>发送前核对两个任务及其版本。任务或关系变化后，请重新选择；循环依赖及任务状态继续由业务规则核对。</Text>
    <Action secondary disabled={disabled || predecessor.loading || edges.loading || !current || !edges.value} onClick={() => { try {
      if (!current || !edges.value) throw new Error('请读取两个任务和当前依赖。');
      onChange(makeTaskDependency(task, current, action, reason, edges.value.items.map(item => item.predecessor_id)));
    } catch (cause) { setError(new MobileError('INPUT_REQUIRED', cause instanceof Error ? cause.message : '请核对依赖资料。')); } }}>将这项明确更新加入请求</Action>
  </View>;
}
