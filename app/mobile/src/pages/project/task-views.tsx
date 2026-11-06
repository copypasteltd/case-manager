import { ScrollView, Text, View } from '@tarojs/components';
import { useState } from 'react';
import { projectSchedule, taskLanes, type TaskView } from '@copypaste/contracts/project-schedule';
import { useRecord } from '../../platform/collection';
import { go } from '../../platform/page';
import type { Schemas } from '../../platform/session';
import { Action, Busy, date, Empty, Fact, Notice, Row, Section, State } from '../../ui';

type Task = Schemas['Task'];
type Props = { items: readonly Task[]; view: TaskView; members: Schemas['ProjectMembers']; hasMore: boolean; filtered: boolean };
const priorityNames: Record<string, string> = { low: '低', normal: '普通', high: '高', urgent: '紧急' };

export function TaskViews({ items, view, members, hasMore, filtered }: Props) {
  function person(id?: string | null) {
    return id ? members.items.find(member => member.id === id)?.display_name ?? '当前成员目录未显示' : '尚未分配';
  }
  return <>
    <View className='cp-pad'><Text className='cp-muted cp-small'>已加载 {items.length} 项{filtered ? '符合筛选条件的' : '可读'}任务。{hasMore ? '仍有后续分页；看板数量和时间范围仅代表已加载内容，请继续加载。' : '当前筛选结果已加载完毕。'}</Text></View>
    {view === 'list' ? <Section title='当前可读任务'>{items.length ? items.map(task => <Row key={task.id} title={task.title} state={task.state} subtitle={(priorityNames[task.priority ?? "normal"] ?? '普通') + '优先级 · ' + person(task.assignee_id) + (task.parent_id ? ' · 子任务' : '')} onClick={() => go('task', task.id)} />) : <Empty>没有符合条件的可读任务。</Empty>}</Section>
      : view === 'board' ? <Section title='任务状态看板'><ScrollView scrollX className='cp-task-board-scroll'><View className='cp-task-board'>
        {taskLanes.map(lane => {
          const rows = items.filter(task => task.state === lane.key);
          return <View key={lane.key} className='cp-task-column'><Text className='cp-task-column-title'>{lane.name} · 已加载 {rows.length} 项</Text>{rows.length ? rows.map(task => <View className='cp-task-card' key={task.id}><Action secondary onClick={() => go('task', task.id)}>{task.title}</Action><Text className='cp-muted cp-small'>{person(task.assignee_id)}{task.parent_id ? ' · 子任务' : ''}</Text><Text className='cp-muted cp-small'>截止 {date(task.due_at)}</Text></View>) : <Text className='cp-muted cp-small'>当前已加载内容无此状态任务。</Text>}</View>;
        })}
      </View></ScrollView></Section> : <TaskTimeline items={items} person={person} />}
  </>;
}

function TaskTimeline({ items, person }: { items: readonly Task[]; person(id?: string | null): string }) {
  const schedule = projectSchedule(items), [expanded, setExpanded] = useState<string>();
  const dated = schedule.rows.filter(row => row.kind !== 'undated' && row.kind !== 'invalid');
  const other = schedule.rows.filter(row => row.kind === 'undated' || row.kind === 'invalid');
  function details(task: Task) {
    return <><Action secondary onClick={() => go('task', task.id)}>{task.title}</Action><Fact label='当前状态'><State value={task.state} /></Fact><Text className='cp-muted cp-small'>{person(task.assignee_id)}</Text><Text className='cp-small'>开始 {date(task.start_at)}{ '\n' }截止 {date(task.due_at)}</Text><Action secondary onClick={() => setExpanded(value => value === task.id ? undefined : task.id)}>{expanded === task.id ? '收起前置任务' : '查看前置任务'}</Action>{expanded === task.id && <TaskPredecessors taskID={task.id} />}</>;
  }
  return <Section title='任务基础时间线'><View className='cp-pad'><Text className='cp-muted cp-small'>按已加载日期绘制，区间代表开始至截止；单点代表只有一个日期或同一时刻，不推断工期和进度。</Text>
    {dated.length ? <><View className='cp-task-axis'><Text className='cp-small'>{date(schedule.min === null ? null : new Date(schedule.min).toISOString())}</Text><Text className='cp-small'>{date(schedule.max === null ? null : new Date(schedule.max).toISOString())}</Text></View>{dated.map(row => <View key={row.task.id} className='cp-task-schedule-row'>{details(row.task)}<Text className='cp-muted cp-small'>{row.kind === 'range' ? '计划区间' : row.kind === 'start-only' ? '仅开始日期' : row.kind === 'due-only' ? '仅截止日期' : '同一时间点'}</Text><View className='cp-task-track'><View className={'cp-task-mark ' + (row.kind === 'range' ? 'cp-task-range' : 'cp-task-point')} style={{ left: row.left + '%', ...(row.kind === 'range' ? { width: row.width + '%' } : {}) }} /></View></View>)}</> : <Empty>当前已加载任务尚无可绘制的有效日期。</Empty>}
    {!!other.length && <Text className='cp-task-column-title'>未排期及日期待核对</Text>}{other.map(row => <View key={row.task.id} className='cp-task-schedule-row'>{details(row.task)}<Text className='cp-muted cp-small'>{row.kind === 'invalid' ? '日期需核对，未绘制为计划区间。' : '尚未设置开始或截止日期。'}</Text></View>)}
  </View></Section>;
}

function TaskPredecessors({ taskID }: { taskID: string }) {
  const value = useRecord<Schemas['TaskDependencies']>('/tasks/' + taskID + '/dependencies');
  return <><Notice error={value.error} />{value.loading ? <Busy /> : value.error ? <Action secondary onClick={value.reload}>重新读取前置任务</Action> : value.value?.items.length ? value.value.items.map(task => <Row key={task.predecessor_id} title={task.title} state={task.state} onClick={() => go('task', task.predecessor_id)} />) : <Text className='cp-muted cp-small'>未读取到可见前置任务。</Text>}</>;
}
