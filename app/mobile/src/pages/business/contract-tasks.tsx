import { Text, View } from '@tarojs/components';
import { useRef, useState } from 'react';
import { contractTaskInput, contractTaskReceipt, type ContractTask } from '@copypaste/contracts/contract-tasks';
import { useCollection } from '../../platform/collection';
import { useCommand, CommandNotice, MFAConfirmation } from '../../platform/command';
import { go } from '../../platform/page';
import type { Schemas } from '../../platform/session';
import { Action, Busy, Empty, Notice, Row, Section, State } from '../../ui';
import { ChoiceList, TextField } from './controls';

type Contract = Schemas['Contract'];
type Node = Schemas['ContractMilestone'];
type Event = Schemas['ContractMilestoneTaskEvent'];
export function MobileContractTaskLinks({ contract, node, me }: { contract: Contract; node: Node; me: Schemas['Me'] }) {
  const [open, setOpen] = useState(false);
  return <View><Action secondary onClick={() => setOpen(v => !v)}>{open ? '收起' : '查看'}节点关联任务</Action>{open && <Links contract={contract} node={node} me={me} />}</View>;
}
function Links({ contract, node, me }: { contract: Contract; node: Node; me: Schemas['Me'] }) {
  const [history, setHistory] = useState(false), [form, setForm] = useState<{ root?: Event }>();
  const path = '/contracts/' + contract.id + '/milestones/' + node.id + '/task-links';
  const rows = useCollection<Event>(path + '?limit=20&history=' + history);
  if (form) return <LinkForm key={form.root?.id ?? 'new'} contract={contract} node={node} root={form.root} path={path} done={() => { setForm(undefined); rows.reload(); }} cancel={() => setForm(undefined)} />;
  return <Section title='节点关联任务'><View className='cp-pad'><Text>仅展示当前有权查阅的任务。关联记录固定原修订；当前进度单独显示，不自动改变合同履行或收付款。</Text><Notice error={rows.error} />
    <Action secondary onClick={() => setHistory(v => !v)}>{history ? '查看当前关联' : '查看关联及撤销历史'}</Action>
    {rows.loading ? <Busy /> : !rows.error && (rows.items.length ? rows.items.map(row => <View className='cp-pad' key={row.id}><Text>{row.action === 'revoke' ? '撤销关联' : row.revoked ? '已撤销关联' : '有效关联'} · {row.task_title}</Text><Text className='cp-paragraph'>固定任务修订 {row.task_version} · <State value={row.task_state} /></Text>{row.current_task && <><Text>当前修订 {row.current_task.version} · <State value={row.current_task.state} /></Text><Row title={row.current_task.title} subtitle='查阅当前任务及其依赖' onClick={() => go('task', row.task_id)} /></>}<Text className='cp-paragraph'>{row.reason}</Text>{row.cross_scope && <Text>已明确核对合同与任务的不同范围</Text>}{row.action === 'link' && !row.revoked && row.current_task && me.actions.includes('contract.execute') && <Action secondary onClick={() => setForm({ root: row })}>核对并撤销此关联</Action>}</View>) : <Empty>没有当前可查阅的关联任务。</Empty>)}
    {rows.next && <Action secondary loading={rows.moreBusy} onClick={() => void rows.more()}>加载更多记录</Action>}
    <Action secondary onClick={rows.reload}>刷新关联记录</Action>{contract.lifecycle === 'effective' && me.actions.includes('contract.execute') && <Action onClick={() => setForm({})}>选择任务并关联此节点</Action>}
  </View></Section>;
}
function LinkForm({ contract, node, root, path, done, cancel }: { contract: Contract; node: Node; root?: Event; path: string; done(): void; cancel(): void }) {
  const [project, setProject] = useState(contract.project_id ?? ''), [task, setTask] = useState<ContractTask | undefined>(root?.current_task ?? undefined);
  const [pick, setPick] = useState<'project' | 'task'>(), [reason, setReason] = useState(''), [ack, setAck] = useState(false), [fresh, setFresh] = useState(false), [error, setError] = useState<unknown>();
  const intent = useRef<{ body: Schemas['ContractMilestoneTaskInput']; task: ContractTask }>();
  const command = useCommand<Event>(done, 'contract-task:' + contract.id + ':' + node.id + ':' + contract.version + ':' + (root?.id ?? 'new'), value => !!intent.current && contractTaskReceipt(value, contract, node, intent.current.task, intent.current.body, root?.id));
  const locked = command.busy || command.blocked;
  if (!locked && pick === 'project') return <ChoiceList<Schemas['Project']> title='选择任务所在项目' path='/projects?limit=20' label={v => v.name} onSelect={v => { setProject(v.id); setTask(undefined); setPick(undefined); }} onClose={() => setPick(undefined)} />;
  if (!locked && pick === 'task') return <ChoiceList<Schemas['Task']> title='选择准确任务修订' path={'/projects/' + project + '/tasks?limit=20'} label={v => v.title + ' · 修订 ' + v.version} onSelect={v => { setTask(v); setPick(undefined); }} onClose={() => setPick(undefined)} />;
  async function submit() {
    try { if (!task) throw new Error('请先选择任务。'); const body = contractTaskInput(contract, node, task, reason, ack); intent.current = { task, body }; setError(undefined); await command.submit(path + (root ? '/' + root.id + '/revoke' : ''), body); }
    catch (cause) { setError(cause); }
  }
  return <Section title={root ? '撤销准确节点任务关联' : '关联节点任务'}><View className='cp-pad'><Text>{node.title} · 固定合同修订 {contract.version}</Text>{!root && <><Action secondary disabled={locked} onClick={() => setPick('project')}>选择项目</Action><Action secondary disabled={locked || !project} onClick={() => setPick('task')}>选择任务</Action></>}{task && <><Text className='cp-paragraph'>{task.title} · 修订 {task.version} · <State value={task.state} /></Text>{contract.project_id !== task.project_id && <Action secondary disabled={locked} onClick={() => setAck(v => !v)}>{ack ? '已核对' : '待核对'}：两侧范围不同，已核实关联用途及独立权限</Action>}</>}<TextField name='task-link-reason' label='本次操作依据' value={reason} onChange={setReason} disabled={locked} /><Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} /><MFAConfirmation disabled={locked} onFresh={setFresh} /><Action disabled={locked || !task || !fresh} loading={command.busy} onClick={() => void submit()}>确认{root ? '撤销关联' : '关联任务'}</Action><Action secondary disabled={command.busy} onClick={cancel}>返回并核对当前记录</Action></View></Section>;
}
