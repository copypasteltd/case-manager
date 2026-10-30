import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { ArrowUpRight, RefreshCw } from 'lucide-react';
import { api, type Schemas } from './api';
import { Busy, date, ErrorNotice } from './components';
import './project-overview.css';
import { ProjectBusinessHub } from './ProjectBusinessHub';

type Overview = Schemas['ProjectOverview'];
type Group = Schemas['ProjectOverviewGroup'];
type Kind = Group['kind'];
const labels: Record<Kind, string> = {
  tasks: '任务', milestones: '里程碑', deliverables: '交付', risks: '风险', issues: '问题', changes: '变更', budgets: '预算修订',
  files: '文件', posts: '知识与日志', contracts: '合同', purchases: '采购', claims: '报销', costs: '实际费用', payments: '收付款', subscriptions: '订阅', services: '服务配置',
};
const stateLabels: Record<string, string> = {
  todo: '待开始', in_progress: '进行中', in_review: '待验收', done: '已完成', blocked: '受阻', cancelled: '已取消',
  open: '待处理', achieved: '已达成', missed: '已错过', mitigating: '应对中', closed: '已关闭', realized: '已转问题', investigating: '调查中', resolved: '已解决',
  draft: '草稿', submitted: '已提交', accepted: '已验收', changes_requested: '待修改', requested: '已申请', implemented: '已实施', rejected: '已驳回', withdrawn: '已撤回',
  approved: '已批准', superseded: '已取代', active: '使用中', published: '已发布', archived: '已归档', confirmed: '已确认', reversed: '已冲销', recorded: '已登记',
  ordering: '下单中', receiving: '收货中', completed: '已完成', terminated: '已终止', paused: '已暂停', signed: '已签署', performing: '履约中', returned: '已退回',
};
const projectKinds = new Set<Kind>(['tasks', 'milestones', 'deliverables', 'risks', 'issues', 'changes', 'budgets']);

export function overviewListURL(pid: string, kind: Kind): string | null {
  if (projectKinds.has(kind)) return '/projects/' + pid + '?tab=' + (kind === 'budgets' ? 'budgets' : kind);
  // Services have individual stable links; the global service list does not yet
  // support a project filter, so it is deliberately not advertised as one.
  if (kind === 'services') return null;
  return '/' + (kind === 'posts' ? 'knowledge' : kind) + '?project_id=' + pid;
}
export function overviewItemURL(pid: string, kind: Kind, id: string): string {
  if (kind === 'tasks') return '/projects/' + pid + '?tab=tasks&task=' + id;
  if (kind === 'changes') return '/changes/' + id;
  if (projectKinds.has(kind)) return overviewListURL(pid, kind)!;
  return '/' + (kind === 'posts' ? 'knowledge' : kind) + '/' + id;
}

// Decimal strings stay strings, including values outside JavaScript's integer
// precision. Currency precision and the original six-place accounting facts
// remain visible; no FX conversion or rounding is implied.
export function projectAmount(value: string): string {
  if (!/^-?\d+(\.\d+)?$/.test(value)) return '金额待核实';
  const [whole, fraction] = value.split('.');
  return whole.replace(/\B(?=(\d{3})+(?!\d))/g, ',') + (fraction === undefined ? '' : '.' + fraction);
}

function snapshotTime(value: string): string {
  const instant = new Date(value);
  return Number.isFinite(instant.getTime()) ? new Intl.DateTimeFormat('zh-HK', { timeZone: 'Asia/Hong_Kong', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(instant) : '未核实';
}

function OverviewCard({ group, projectID }: { group: Group; projectID: string }) {
  const url = overviewListURL(projectID, group.kind);
  return <article className="panel overview-card" data-overview-kind={group.kind} aria-label={labels[group.kind]}>
    <div className="section-heading compact"><h3>{labels[group.kind]}</h3><span className="overview-count">{group.available ? group.count : '—'}</span></div>
    {!group.available ? <p className="muted">未核实 · 当前读取范围不足</p> : <>
      <p className="overview-scope">当前可读记录{group.kind === 'files' ? ' · 有效目录中的文件' : group.kind === 'posts' ? ' · 不含已归档' : ''}</p>
      {!!group.states.length && <ul className="overview-states" aria-label="可读记录状态">{group.states.map(state => <li key={state.state}>{stateLabels[state.state] ?? state.state}<strong>{state.count}</strong></li>)}</ul>}
      {group.items.length ? <ul className="overview-items">{group.items.map(item => <li key={item.id}><Link to={overviewItemURL(projectID, group.kind, item.id)}><span>{item.title || '未命名记录'}</span><ArrowUpRight size={14} /></Link><small>{stateLabels[item.state] ?? item.state} · {date(item.updated_at)}</small></li>)}</ul> : <p className="muted">暂无可读记录。</p>}
      {group.has_more && <p className="form-note">仅预览最近 {group.items.length} 项，共 {group.count} 项可读记录。</p>}
      {url && <Link className="text-button" to={url}>查看{labels[group.kind]}<ArrowUpRight size={14} /></Link>}
      {group.kind === 'services' && <p className="form-note">这里展示服务配置；打开服务后核对当前健康状态。</p>}
      {group.kind === 'payments' && <p className="form-note">包含本项目资料、费用分配与合同现金归属；登记金额不计入发生支出。</p>}
    </>}
  </article>;
}

export function ProjectOverviewContent({ projectID, data, pending, error, refreshing, onRefresh }: {
  projectID: string; data?: Overview; pending: boolean; error: unknown; refreshing: boolean; onRefresh: () => void;
}) {
  // A failed fresh read never falls back to previously authorized cached facts.
  const current = !error && data?.project_id === projectID ? data : undefined;
  return <div className="project-overview">
    <div className="section-heading"><div><h2>项目业务概览</h2><p className="muted">仅汇总当前可读记录，供日常跟进。结项核查单独执行。</p></div><button className="button secondary" onClick={onRefresh} disabled={refreshing}><RefreshCw size={16} />{refreshing ? '正在刷新…' : '刷新概览'}</button></div>
    <ErrorNotice error={error} />
    {pending ? <Busy label="正在汇总项目业务" /> : !current ? <p className="muted">概览未核实，请重新读取。</p> : <>
      <p className="form-note">快照时间 {snapshotTime(current.as_of)} · 香港时间 · 项目版本 {current.project_version}。无权读取的内容不计入统计。</p>
      <section className="panel overview-finance" aria-label="项目财务概览">
        <div><span className="eyebrow">PROJECT FINANCIAL FACTS</span><h3>本项目发生支出</h3><p className="muted">全期间、本项目实际分摊；费用与冲销逐币种汇总。草稿、付款、审批和订阅预测分别核对。</p></div>
        {!current.incurred.available ? <p className="muted">发生支出未核实 · 当前报表读取范围不足</p> : <>
          {current.incurred.totals.length ? <div className="table-scroll"><table className="data-table"><caption className="sr-only">当前可读费用在本项目的发生支出，按原币列示</caption><thead><tr><th>币种</th><th>费用原额</th><th>冲销原额</th><th>净发生额</th><th>记账条目</th></tr></thead><tbody>{current.incurred.totals.map(total => <tr key={total.currency}><th scope="row">{total.currency}</th><td>{projectAmount(total.expense_amount)}</td><td>{projectAmount(total.reversal_amount)}</td><td><strong>{projectAmount(total.net_amount)}</strong></td><td>{total.entry_count}</td></tr>)}</tbody></table></div> : <p className="muted">暂无当前可读的已确认发生支出。</p>}
          <p className="form-note">未确认费用 {current.incurred.unconfirmed_count} 项，未计入金额。冲销保留原费用与冲销记录，净额抵销。</p>
        </>}
        <div className="overview-budget"><h3>当前批准预算</h3>{!current.groups.find(g => g.kind === 'budgets')?.available ? <p className="muted">预算未核实 · 当前读取范围不足</p> : current.budgets.length ? <ul>{current.budgets.map(budget => <li key={budget.budget_id}><Link to={'/projects/' + projectID + '?tab=budgets'}>{budget.currency} 修订 {budget.revision}</Link><strong>{projectAmount(budget.amount)}</strong></li>)}</ul> : <p className="muted">暂无当前批准预算。</p>}<p className="form-note">预算按其批准期间独立展示；不同币种不相加，也不据此推算剩余预算。</p></div>
      </section>
      <div className="overview-grid">{current.groups.map(group => <OverviewCard key={group.kind} group={group} projectID={projectID} />)}</div>
    </>}
  </div>;
}

export default function ProjectOverview({ projectID }: { projectID: string }) {
  const query = useQuery({ queryKey: ['project-overview', projectID], queryFn: ({ signal }) => api<Overview>('/projects/' + projectID + '/overview', { signal }), refetchInterval: 60000 });
  return <><ProjectBusinessHub projectID={projectID} /><ProjectOverviewContent projectID={projectID} data={query.data} pending={query.isPending} error={query.error} refreshing={query.isFetching} onRefresh={() => void query.refetch()} /></>;
}
