import { useState, type FormEvent } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';
import { ArrowRight, CheckCircle2, GitBranch, Plus, RefreshCw, Search } from 'lucide-react';
import { api, type Schemas, type Task } from './api';
import { Busy, date, Empty, ErrorNotice, State } from './components';
import { projectSchedule, scheduleTime, taskLanes, taskView } from './projectSchedule';
import './project-tasks.css';

const priorities: Record<string, string> = { low: '低', normal: '普通', high: '高', urgent: '紧急' };
type Props = {
  items: Task[]; state: string; search: string; pending: boolean; error: unknown;
  hasMore: boolean; loadingMore: boolean; refreshing: boolean; canCreate: boolean;
  userName: (id?: string | null) => string;
  onState: (value: string) => void; onSearch: (value: string) => void;
  onLoadMore: () => void; onRefresh: () => void; onCreate: () => void; onSelect: (id: string) => void;
};

export default function ProjectTaskViews(props: Props) {
  const [params, setParams] = useSearchParams();
  const view = taskView(params.get('view'));
  function choose(next: string) {
    setParams(current => { const result = new URLSearchParams(current); result.set('view', next); return result; }, { replace: true });
  }
  function search(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); props.onSearch(String(new FormData(event.currentTarget).get('q') ?? '').trim());
  }
  // Refetch errors can mean access was revoked. Do not present cached rows as
  // current business facts until an actual successful read has happened again.
  const items = props.error ? [] : props.items;
  return <>
    <div className="project-task-toolbar">
      <div className="task-view-selector" role="group" aria-label="任务呈现方式">
        {([['list', '列表'], ['board', '看板'], ['timeline', '时间线']] as const).map(([key, label]) => <button className="button secondary" key={key} aria-pressed={view === key} onClick={() => choose(key)}>{label}</button>)}
      </div>
      <div className="task-toolbar-actions">
        <button className="button secondary" onClick={props.onRefresh} disabled={props.refreshing}><RefreshCw size={15} />{props.refreshing ? '正在刷新…' : '刷新任务'}</button>
        {props.canCreate && <button className="button primary" onClick={props.onCreate}><Plus size={16} />新建任务</button>}
      </div>
    </div>
    <div className="toolbar task-filters">
      <form className="search-field" onSubmit={search}><Search size={17} /><input key={props.search} name="q" aria-label="搜索任务标题" defaultValue={props.search} placeholder="搜索任务标题" maxLength={200} /><button type="submit" aria-label="搜索任务"><ArrowRight size={17} /></button></form>
      <div className="filter-field"><select aria-label="任务状态" value={props.state} onChange={event => props.onState(event.target.value)}><option value="">全部任务</option>{taskLanes.map(lane => <option key={lane.key} value={lane.key}>{lane.name}</option>)}</select></div>
    </div>
    <ErrorNotice error={props.error} />
    {!props.pending && !props.error && <p className="form-note task-scope" role="status">已加载 {items.length} 项{props.state || props.search ? '符合筛选条件的' : '可读'}任务。{props.hasMore ? '还有任务未加载，当前视图与分列数量只涵盖已加载部分。' : '本次查询没有后续页。'}{props.search && <span> 标题：{props.search}</span>}</p>}
    {props.pending ? <Busy label="正在读取任务" /> : props.error ? <button className="button secondary" onClick={props.onRefresh} disabled={props.refreshing}>重新读取任务</button> : !items.length ? <div className="panel"><Empty title={props.state || props.search ? '没有符合条件的任务' : '把目标拆成下一步'} detail={props.state || props.search ? '尝试其他标题或状态。' : '清晰的任务，让每个人都知道接下来要做什么。'} action={props.canCreate && !props.state && !props.search ? <button className="button primary" onClick={props.onCreate}><Plus size={16} />新建任务</button> : undefined} /></div> :
      view === 'board' ? <TaskBoard items={items} state={props.state} userName={props.userName} onSelect={props.onSelect} /> :
      view === 'timeline' ? <TaskTimeline items={items} userName={props.userName} onSelect={props.onSelect} /> :
      <div className="panel table-panel"><div className="table-scroll" tabIndex={0} aria-label="项目任务"><table><thead><tr><th>任务</th><th>状态</th><th>负责人</th><th>截止时间</th><th>验收</th></tr></thead><tbody>{items.map(task => <tr key={task.id}><td><button className="task-title" onClick={() => props.onSelect(task.id)}><span className={'task-check ' + (task.state === 'done' ? 'is-done' : '')}><CheckCircle2 size={19} /></span><strong>{task.title}</strong></button></td><td><State state={task.state} /></td><td>{props.userName(task.assignee_id)}</td><td className="muted">{date(task.due_at)}</td><td className="muted">{task.review_required ? '需验收' : '直接完成'}</td></tr>)}</tbody></table></div></div>}
    {props.hasMore && !props.error && <div className="list-footer"><span>继续读取后续任务</span><button className="button secondary" disabled={props.loadingMore} onClick={props.onLoadMore}>{props.loadingMore ? '正在读取…' : '加载更多任务'}</button></div>}
  </>;
}

function TaskBoard({ items, state, userName, onSelect }: Pick<Props, 'items' | 'state' | 'userName' | 'onSelect'>) {
  return <div className="task-board-scroll" role="region" tabIndex={0} aria-label="项目任务看板"><div className="task-board">
    {taskLanes.filter(lane => !state || lane.key === state).map(lane => {
      const tasks = items.filter(task => task.state === lane.key);
      return <section className="task-board-lane" key={lane.key} aria-label={lane.name + '任务列'}><header><h3>{lane.name}</h3><small>已加载 {tasks.length} 项</small></header>
        {tasks.length ? tasks.map(task => <article className="task-board-card" key={task.id}><button className="task-card-title" onClick={() => onSelect(task.id)}>{task.title}</button><div className="task-card-facts"><span>{userName(task.assignee_id)}</span><span>{priorities[task.priority ?? 'normal'] ?? task.priority}优先级</span></div><p className="muted">截止 {date(task.due_at)}</p>{(task.parent_id || task.review_required) && <small className="muted">{[task.parent_id ? '子任务' : '', task.review_required ? '需要验收' : ''].filter(Boolean).join(' · ')}</small>}</article>) : <p className="muted task-lane-empty">当前已加载部分暂无此状态任务。</p>}
      </section>;
    })}
  </div></div>;
}

function TaskTimeline({ items, userName, onSelect }: Pick<Props, 'items' | 'userName' | 'onSelect'>) {
  const [expanded, setExpanded] = useState<string | null>(null);
  const schedule = projectSchedule(items);
  const dated = schedule.rows.filter(row => row.kind !== 'invalid' && row.kind !== 'undated');
  const other = schedule.rows.filter(row => row.kind === 'invalid' || row.kind === 'undated');
  return <div className="task-timeline-panel panel">
    <div className="task-timeline-help"><h3>基础时间线</h3><p className="muted">时间以香港时区显示；横条表示已设置的计划区间，圆点表示单个时间或仅设置一个日期。缺少的日期保持未设置。</p></div>
    {dated.length ? <div className="task-timeline-scroll" role="region" tabIndex={0} aria-label="项目任务时间线"><div className="task-timeline-grid">
      <div className="task-timeline-axis"><span>任务与期限</span><div><span>{scheduleTime(schedule.min)}</span><span>{scheduleTime(schedule.max)}</span></div></div>
      {dated.map(row => <div className="task-timeline-row" key={row.task.id}>
        <div className="task-timeline-facts"><button className="task-card-title" onClick={() => onSelect(row.task.id)}>{row.task.title}</button><div className="task-card-facts"><State state={row.task.state} /><span>{userName(row.task.assignee_id)}</span></div><small>开始 {scheduleTime(row.task.start_at)}<br />截止 {scheduleTime(row.task.due_at)}</small><button className="text-button" aria-expanded={expanded === row.task.id} onClick={() => setExpanded(value => value === row.task.id ? null : row.task.id)}><GitBranch size={14} />{expanded === row.task.id ? '收起' : '查看'}前置任务</button></div>
        <div className="task-timeline-track" role="img" aria-label={`${row.task.title}：${row.kind === 'range' ? '计划区间' : row.kind === 'start-only' ? '仅开始日期' : row.kind === 'due-only' ? '仅截止日期' : '同一时间点'}；开始${scheduleTime(row.task.start_at)}，截止${scheduleTime(row.task.due_at)}`}><span className={'task-schedule-mark ' + (row.kind === 'range' ? 'range' : 'point')} style={{ left: row.left + '%', ...(row.kind === 'range' ? { width: row.width + '%' } : {}) }} /></div>
        {expanded === row.task.id && <div className="task-timeline-dependencies"><DependencyFacts taskID={row.task.id} onSelect={onSelect} /></div>}
      </div>)}
    </div></div> : <p className="muted task-timeline-help">当前已加载任务尚无可绘制的有效日期。</p>}
    {!!other.length && <section className="task-undated" aria-label="未排期及日期待核对任务"><h3>未排期及日期待核对</h3>{other.map(row => <div className="task-undated-row" key={row.task.id}><div><button className="task-card-title" onClick={() => onSelect(row.task.id)}>{row.task.title}</button><small>{row.kind === 'invalid' ? '日期需核对，未绘制为计划区间' : '尚未设置开始或截止日期'}</small></div><State state={row.task.state} /><button className="text-button" aria-expanded={expanded === row.task.id} onClick={() => setExpanded(value => value === row.task.id ? null : row.task.id)}><GitBranch size={14} />{expanded === row.task.id ? '收起' : '查看'}前置任务</button>{expanded === row.task.id && <div className="task-timeline-dependencies"><DependencyFacts taskID={row.task.id} onSelect={onSelect} /></div>}</div>)}</section>}
  </div>;
}

function DependencyFacts({ taskID, onSelect }: { taskID: string; onSelect: (id: string) => void }) {
  const query = useQuery({ queryKey: ['dependencies', taskID], queryFn: ({ signal }) => api<Schemas['TaskDependencies']>('/tasks/' + taskID + '/dependencies', { signal }), staleTime: 0 });
  return <><ErrorNotice error={query.error} />{query.isPending || query.isFetching ? <Busy label="正在读取前置任务" /> : query.error ? <button className="text-button" onClick={() => void query.refetch()}>重新读取前置任务</button> : <><p className="muted">当前可读的前置任务；前置完成后才能推进后续任务。</p>{query.data?.items.length ? query.data.items.map(item => <div className="dependency-row" key={item.predecessor_id}><button className="text-button" onClick={() => onSelect(item.predecessor_id)}>{item.title}</button><State state={item.state} /></div>) : <p className="muted">未读取到可见前置任务。</p>}</>}</>;
}
