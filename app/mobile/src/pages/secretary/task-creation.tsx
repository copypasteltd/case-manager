import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { makeTaskCreation, type Mutation } from '@copypaste/contracts/mutation';
import { MobileError, type Schemas } from '../../platform/session';
import { Action, Fact, Notice } from '../../ui';
import { ChoiceList, SelectField, TextField } from '../business/controls';

export function ConversationTaskCreation({ project, members, disabled, onChange }: {
  project: Schemas['Project']; members?: Schemas['ProjectMembers']; disabled: boolean; onChange(value: Mutation): void;
}) {
  const [title, setTitle] = useState(''), [body, setBody] = useState(''), [assignee, setAssignee] = useState('');
  const [parent, setParent] = useState<Schemas['Task']>(), [milestone, setMilestone] = useState<Schemas['Milestone']>();
  const [picking, setPicking] = useState(''), [priority, setPriority] = useState(''), [review, setReview] = useState('');
  const [start, setStart] = useState(''), [due, setDue] = useState(''), [startMode, setStartMode] = useState('clear'), [dueMode, setDueMode] = useState('clear');
  const [reason, setReason] = useState(''), [error, setError] = useState<MobileError>();
  if (picking === 'parent') return <ChoiceList<Schemas['Task']> title='选择父任务' path={'/projects/' + project.id + '/tasks?limit=20'} selected={parent?.id} label={t => t.title} eligible={t => !['done', 'cancelled'].includes(t.state)} onSelect={t => { if (!disabled) { setParent(t); setPicking(''); } }} onClose={() => setPicking('')} />;
  if (picking === 'milestone') return <ChoiceList<Schemas['Milestone']> title='选择关联里程碑' path={'/projects/' + project.id + '/milestones?limit=20'} selected={milestone?.id} label={m => m.title} eligible={m => m.state === 'open' || m.state === 'missed'} onSelect={m => { if (!disabled) { setMilestone(m); setPicking(''); } }} onClose={() => setPicking('')} />;
  return <View>
    <Notice error={error} />
    <TextField name='task-create-title' label='任务标题' value={title} max={400} disabled={disabled} onChange={setTitle} />
    <TextField name='task-create-body' label='完整任务说明' multiline value={body} max={40000} disabled={disabled} onChange={setBody} />
    <SelectField label='任务负责人' value={assignee} disabled={disabled} onChange={setAssignee} options={[[ '', '明确暂不分配负责人' ], ...(members?.items.filter(m => m.status === 'active').map(m => [m.id, m.display_name] as const) ?? [])]} />
    <Fact label='父任务'><Text>{parent?.title ?? '独立任务，不设置父任务'}</Text></Fact>
    <Action secondary disabled={disabled} onClick={() => setPicking('parent')}>选择父任务</Action>
    {parent && <Action secondary disabled={disabled} onClick={() => setParent(undefined)}>明确不设置父任务</Action>}
    <Fact label='关联里程碑'><Text>{milestone?.title ?? '明确不关联里程碑'}</Text></Fact>
    <Action secondary disabled={disabled} onClick={() => setPicking('milestone')}>选择关联里程碑</Action>
    {milestone && <Action secondary disabled={disabled} onClick={() => setMilestone(undefined)}>明确不关联里程碑</Action>}
    <SelectField label='开始时刻处理' value={startMode} disabled={disabled} onChange={setStartMode} options={[[ 'clear', '明确不设置' ], ['set', '设置准确时刻']]} />
    {startMode === 'set' && <TextField name='task-create-start' label='开始时刻' value={start} disabled={disabled} onChange={setStart} hint='例如2027-03-05T10:00:00.123456+08:00，含时区，最多六位小数秒' />}
    <SelectField label='截止时刻处理' value={dueMode} disabled={disabled} onChange={setDueMode} options={[[ 'clear', '明确不设置' ], ['set', '设置准确时刻']]} />
    {dueMode === 'set' && <TextField name='task-create-due' label='截止时刻' value={due} disabled={disabled} onChange={setDue} />}
    <SelectField label='优先级' value={priority} disabled={disabled} onChange={setPriority} options={[[ '', '请明确选择优先级' ], ['low', '低'], ['normal', '普通'], ['high', '高'], ['urgent', '紧急']]} />
    <SelectField label='需要人工验收' value={review} disabled={disabled} onChange={setReview} options={[[ '', '请明确选择验收要求' ], ['true', '需要人工验收'], ['false', '无需人工验收']]} />
    <TextField name='task-create-reason' label='本次创建原因' value={reason} max={400} disabled={disabled} onChange={setReason} />
    <Text className='cp-small cp-paragraph'>请核对全部九项资料。发送后只创建一个待处理任务，新版本从1开始。父任务表示子任务归属；前置依赖和人工验收请在任务页面办理。</Text>
    <Action secondary disabled={disabled} onClick={() => { try {
      if (review !== 'true' && review !== 'false') throw new Error('请明确选择人工验收要求。');
      onChange(makeTaskCreation(project, { title, body, assignee_id: assignee || null, parent_id: parent?.id ?? null, milestone_id: milestone?.id ?? null, start_at: startMode === 'clear' ? null : start, due_at: dueMode === 'clear' ? null : due, priority, review_required: review === 'true' }, reason));
    } catch (cause) { setError(new MobileError('INPUT_REQUIRED', cause instanceof Error ? cause.message : '请完整核对任务资料。')); } }}>将这项明确更新加入请求</Action>
  </View>;
}
