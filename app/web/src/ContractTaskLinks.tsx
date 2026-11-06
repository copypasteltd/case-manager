import { useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { contractTaskInput, contractTaskReceipt, type ContractTask } from '@copypaste/contracts/contract-tasks';
import { api, ApiError, type Page, type Schemas } from './api';
import { Busy, Dialog, Empty, ErrorNotice, Field, State, date } from './components';
import { ProjectReferenceSelect } from './ProjectReferenceSelect';
import { MFAConfirmation } from './MFAConfirmation';

type Contract = Schemas['Contract'];
type Node = Schemas['ContractMilestone'];
type Event = Schemas['ContractMilestoneTaskEvent'];
const endpoint = (contract: Contract, node: Node) => '/contracts/' + contract.id + '/milestones/' + node.id + '/task-links';

export function ContractTaskLinks({ contract, node, me }: { contract: Contract; node: Node; me: Schemas['Me'] }) {
  const [open, setOpen] = useState(false), [history, setHistory] = useState(false), [form, setForm] = useState<{ root?: Event }>(), [receipt, setReceipt] = useState<Event>();
  const rows = useInfiniteQuery({ queryKey: ['contract-task-links', contract.id, node.id, history, me.user.id, me.authorization_version], enabled: open,
    gcTime: 0, retry: false, initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Event>>(endpoint(contract, node) + '?limit=20&history=' + history + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }),
    getNextPageParam: page => page.next_cursor ?? undefined });
  const items = rows.error ? [] : [...new Map((rows.data?.pages.flatMap(page => page.items) ?? []).map(row => [row.id, row])).values()];
  return <div className='contract-attachment'><button className='button secondary' type='button' onClick={() => setOpen(value => !value)}>{open ? '收起关联任务' : '查看关联任务'}</button>
    {open && <><p className='form-note'>仅显示当前有权查阅的任务。关联保留核对时版本，任务后续进度另行显示；关联不改变审批、交付、费用、收付款或履约结论。</p>
      <label className='checkbox-field'><input type='checkbox' checked={history} onChange={event => setHistory(event.target.checked)} />显示关联与撤销历史</label><ErrorNotice error={rows.error} />
      {receipt && <p className='form-note' role='status'>{receipt.action === 'link' ? '任务关联已保存' : '准确关联已撤销'} · 来源任务 v{receipt.task_version}。</p>}
      {rows.isPending ? <Busy label='正在读取节点任务关联' /> : !rows.error && (items.length ? items.map(row => <article key={row.id} className='payment-plan-item'><strong>{row.action === 'revoke' ? '撤销关联：' : row.revoked ? '历史关联：' : '有效关联：'}{row.task_title}</strong><p>核对时 v{row.task_version} · 当前 v{row.current_task?.version ?? '不可查阅'} · {date(row.created_at)}</p>
        {row.current_task && <><p>{row.current_task.title} · <State state={row.current_task.state} /></p><Link className='text-button' to={'/projects/' + row.current_task.project_id + '?task=' + row.task_id}>查阅当前任务</Link></>}<p>{row.reason}</p>{row.cross_scope && <p className='form-note'>此记录包含已明确确认的不同范围关联。</p>}
        {row.action === 'link' && !row.revoked && row.current_task && me.actions.includes('contract.execute') && <button type='button' className='button secondary' onClick={() => setForm({ root: row })}>撤销此准确关联</button>}</article>) : <Empty title='暂无关联任务' detail='当前筛选范围暂无可读任务关联。' />)}
      {rows.hasNextPage && <button type='button' className='button secondary' disabled={rows.isFetching} onClick={() => void rows.fetchNextPage()}>继续读取关联记录</button>}<p className='form-note'>已加载 {items.length} 条{rows.hasNextPage ? '，仍有后续分页。' : '，当前可读筛选结果已到末尾。'}</p>
      <button type='button' className='button secondary' disabled={rows.isFetching} onClick={() => void rows.refetch()}>刷新节点任务关联</button>{contract.lifecycle === 'effective' && me.actions.includes('contract.execute') && <button type='button' className='button secondary' onClick={() => setForm({})}>关联已有任务</button>}
    </>}{form && <TaskLinkDialog contract={contract} node={node} me={me} root={form.root} onClose={() => setForm(undefined)} onCommitted={value => { setReceipt(value); setForm(undefined); void rows.refetch(); }} />}
  </div>;
}

function TaskLinkDialog({ contract, node, me, root, onClose, onCommitted }: { contract: Contract; node: Node; me: Schemas['Me']; root?: Event; onClose(): void; onCommitted(value: Event): void }) {
  const [base] = useState({ contract, node }), [projectID, setProjectID] = useState(root?.task_project_id ?? contract.project_id ?? ''), [taskID, setTaskID] = useState('');
  const [reason, setReason] = useState(''), [acknowledge, setAcknowledge] = useState(false), [fresh, setFresh] = useState(false), [locked, setLocked] = useState(false);
  const taskQuery = useQuery({ queryKey: ['contract-task-source', taskID, me.user.id, me.authorization_version], enabled: !root && !!taskID, gcTime: 0, retry: false,
    queryFn: ({ signal }) => api<Schemas['Task']>('/tasks/' + taskID, { signal }) });
  const selected: ContractTask | undefined = root?.current_task ?? (taskQuery.error ? undefined : taskQuery.data);
  const cross = !!selected && base.contract.project_id !== selected.project_id;
  const intent = useRef<{ body: Schemas['ContractMilestoneTaskInput']; task: ContractTask; key: string }>();
  const cache = useQueryClient();
  const mutation = useMutation({ mutationFn: async () => {
    if (!intent.current) {
      if (!selected || taskQuery.isFetching) throw new Error('请先选择并读取当前可读任务。');
      intent.current = { body: contractTaskInput(base.contract, base.node, selected, reason, acknowledge), task: selected, key: crypto.randomUUID() };
      setLocked(true);
    }
    const fixed = intent.current, value = await api<Event>(endpoint(base.contract, base.node) + (root ? '/' + root.id + '/revoke' : ''), { method: 'POST', body: fixed.body, key: fixed.key });
    if (!contractTaskReceipt(value, base.contract, base.node, fixed.task, fixed.body, root?.id)) throw new ApiError(0, 'RESULT_UNCONFIRMED', '回执未能匹配本次核对内容，请以相同操作继续核对。', '');
    return value;
  }, onError: cause => {
    if (cause instanceof ApiError && cause.status >= 400 && cause.status < 500 && !['OPERATION_IN_PROGRESS', 'IDEMPOTENCY_MISMATCH'].includes(cause.code)) { intent.current = undefined; setLocked(false); }
  }, onSuccess: value => { void cache.invalidateQueries({ queryKey: ['contract-task-links', contract.id, node.id] }); onCommitted(value); } });
  const busy = mutation.isPending;
  return <Dialog title={root ? '核对并撤销节点任务关联' : '关联已有任务到准确合同节点'} busy={busy || locked} onClose={onClose}><form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}>
    <h3>{base.contract.title} · {base.node.title}</h3><p className='form-note'>只保存或撤销本次明确选择的关联，保留完整历史；合同修订和任务版本改变后须重新核对。</p>
    {!root && <><Field label='任务所属项目'><ProjectChoice value={projectID} disabled={busy || locked} onChange={value => { setProjectID(value); setTaskID(''); }} me={me} /></Field>{projectID && <ProjectReferenceSelect projectID={projectID} kind='tasks' value={taskID} onChange={setTaskID} me={me} allowTerminal disabled={busy || locked} />}</>}
    <ErrorNotice error={taskQuery.error ?? mutation.error} />{taskQuery.isFetching && <Busy label='正在核对当前任务' />}{selected && <><h4>{selected.title}</h4><p>当前版本 {selected.version} · <State state={selected.state} /></p></>}
    {cross && <label className='checkbox-field'><input type='checkbox' checked={acknowledge} disabled={busy || locked} onChange={event => setAcknowledge(event.target.checked)} />我已核对合同与任务位于不同项目或公司范围，确认两侧用途与查阅权限；此操作不授予任何访问权限。</label>}
    <Field label='本次关联操作依据'><textarea required maxLength={200} value={reason} disabled={busy || locked} onChange={event => setReason(event.target.value)} /></Field>
    <MFAConfirmation onFresh={setFresh} />{locked && <p className='form-note'>本次内容已固定。结果未确认时继续使用相同操作核对，不另建关联。</p>}
    <div className='dialog-actions'><button type='button' className='button secondary' disabled={busy || locked} onClick={onClose}>返回重新核对</button><button className='button primary' disabled={busy || !fresh || !selected || taskQuery.isFetching || (!locked && (!reason.trim() || (cross && !acknowledge)))}>{locked ? '继续核对此操作' : root ? '确认撤销此准确关联' : '确认关联此任务版本'}</button></div>
  </form></Dialog>;
}

function ProjectChoice({ value, onChange, me, disabled }: { value: string; onChange(value: string): void; me: Schemas['Me']; disabled: boolean }) {
  const projects = useInfiniteQuery({ queryKey: ['contract-task-projects', me.user.id, me.authorization_version], initialPageParam: '', gcTime: 0, retry: false,
    queryFn: ({ pageParam, signal }) => api<Page<Schemas['Project']>>('/projects?limit=20' + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const items = projects.error ? [] : projects.data?.pages.flatMap(page => page.items) ?? [];
  return <><ErrorNotice error={projects.error} /><select aria-label='任务所属项目' value={value} disabled={disabled || projects.isFetching || !!projects.error} onChange={event => onChange(event.target.value)}><option value=''>请选择可读项目</option>{value && !items.some(project => project.id === value) && <option value={value}>当前已选项目，任务将单独核对</option>}{items.map(project => <option value={project.id} key={project.id}>{project.name}</option>)}</select>{projects.hasNextPage && <button type='button' className='text-button' disabled={disabled || projects.isFetching} onClick={() => void projects.fetchNextPage()}>继续读取可选项目</button>}</>;
}
