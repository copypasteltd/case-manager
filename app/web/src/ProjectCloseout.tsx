import './project-closeout.css';
import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, ApiError, type Me, type Project, type Schemas } from './api';
import { Busy, date, Dialog, Empty, ErrorNotice, Field } from './components';
import { FileVersionPicker, useCommandKey, type FileReference } from './BusinessSelectors';
import { MFAConfirmation } from './MFAConfirmation';

type Preview = Schemas['ProjectCloseout'];
type Group = Schemas['ProjectCloseoutGroup'];
type Archive = Schemas['ProjectArchive'];
const labels: Record<string, string> = { tasks: '任务', milestones: '里程碑', deliverables: '交付与验收', risks: '风险', issues: '问题', changes: '项目变更', budgets: '预算版本', contracts: '合同与履约', purchases: '采购履约', claims: '报销', costs: '实际费用与结算', payments: '资金与退款', subscriptions: '订阅与账期' };
const stateLabels: Record<string, string> = { todo: '待开始', in_progress: '进行中', blocked: '受阻', in_review: '处理中', done: '已完成', cancelled: '已取消', open: '未完成', achieved: '已达成', missed: '已错过', draft: '草稿', submitted: '待验收', accepted: '已验收', changes_requested: '已退回修改', mitigating: '应对中', closed: '已关闭', realized: '已转问题', investigating: '调查中', resolved: '已解决', approved: '已批准', rejected: '已拒绝', withdrawn: '已撤回', implemented: '已实施', superseded: '已被替代', pending_signature: '待签署', effective: '已生效', completed: '已完成', terminated: '已终止', requested: '待执行', ordering: '订购中', receiving: '收货中', not_submitted: '未提交', returned: '已退回', confirmed: '已确认', reversed: '已冲销', recorded: '已登记', active: '仍在生效', paused: '已暂停', archived: '已归档' };
const explanations: Record<string, string> = {
  tasks: '任务须已完成或取消。', milestones: '里程碑须已达成或明确取消；已错过仍需处置。',
  deliverables: '交付须按可核验版本完成验收，旧资料须先补齐依据。', risks: '风险须关闭或转为问题跟进。', issues: '问题须已关闭。',
  changes: '变更须实施、拒绝或撤回；旧记录须补齐依据。', budgets: '预算展示历史版本，不代表已发生费用或已付款。',
  contracts: '合同须完成或终止，并核对当前履约、结算与补充协议。终止后仍可能存在财务事项。',
  purchases: '采购须完成或取消，验收、变更、实际费用及退款均须处置。',
  claims: '待办申请须处置，获批报销须登记实际费用并结清员工垫付。',
  costs: '费用须已确认；共享费用按完整费用核对，付款净分配须与当前应结金额一致。',
  payments: '资金须已登记或冲销，退款须明确分配或归属合同。',
  subscriptions: '订阅须已取消或归档，取消已生效且有供应方取消凭据，账期预测已处置，实际核对仍有效。',
};
function destination(item: Schemas['ProjectCloseoutItem'], projectID: string) {
  const standalone: Record<string, string> = { contracts: 'contracts', purchases: 'purchases', claims: 'claims', costs: 'costs', payments: 'payments', subscriptions: 'subscriptions', changes: 'changes' };
  if (standalone[item.kind]) return '/' + standalone[item.kind] + '/' + item.id;
  return '/projects/' + projectID + '?tab=' + item.kind + (item.kind === 'tasks' ? '&task=' + item.id : '');
}
function Checklist({ groups, projectID, historical = false }: { groups: Group[]; projectID: string; historical?: boolean }) {
  return <div className="closeout-groups">{groups.map(group => <details className="closeout-group" key={group.kind} open={group.visibility === 'restricted' || (group.open_count ?? 0) > 0}>
    <summary><strong>{labels[group.kind]}</strong><span className={'badge ' + (group.visibility === 'restricted' || group.kind === 'budgets' ? 'neutral' : group.open_count ? 'warning' : 'success')}>
      {group.visibility === 'restricted' ? '权限受限，尚未核实' : group.open_count ? `${group.open_count} 项待处置` : group.kind === 'budgets' ? '参考版本' : '未发现阻塞'}</span>
      {group.count !== null && <small>共 {group.count} 项{historical ? ' · 归档时' : ''}</small>}</summary>
    <p className="muted">{explanations[group.kind]}</p>
    {group.visibility === 'restricted' ? <p className="notice neutral">当前权限无法完整核对本类别。请安排具备相应查阅权限的人员处理。</p> : !group.items.length ? <p className="muted">核查范围内暂无此类记录。</p> : <ul className="closeout-items">{group.items.map(item => <li key={item.id}>
      <div><Link to={destination(item, projectID)} reloadDocument>{item.title || labels[group.kind] + '记录'}</Link><small>版本 {item.version} · {stateLabels[item.state] ?? '状态待核实'}</small></div>
      <span className={'badge ' + (item.blocking ? 'warning' : 'neutral')}>{item.blocking ? '待处置' : group.kind === 'budgets' ? '参考版本' : '未构成阻塞'}</span>
    </li>)}</ul>}
  </details>)}</div>;
}
export default function ProjectCloseout({ project, me, viewer }: { project: Project; me: Me; viewer: boolean }) {
  const cache = useQueryClient();
  const [dialog, setDialog] = useState<'closing' | 'archive' | null>(null);
  const query = useQuery({ queryKey: ['project-closeout', project.id], queryFn: ({ signal }) => api<Preview>('/projects/' + project.id + '/closeout', { signal }) });
  const history = useInfiniteQuery({ queryKey: ['project-archives', project.id], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Schemas['ProjectArchiveList']>('/projects/' + project.id + '/archives?' + new URLSearchParams({ limit: '10', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const refresh = async () => { await Promise.all(['project-closeout', 'project-archives', 'project', 'projects', 'dashboard', 'tasks'].map(key => cache.invalidateQueries({ queryKey: [key] }))); };
  const p = query.error ? undefined : query.data;
  const archives = history.error ? [] : history.data?.pages.flatMap(page => page.items) ?? [];
  return <div className="project-closeout"><div className="section-heading compact"><div><h2>结项与归档</h2><p className="muted">逐项处理未结事项，记录复盘，再由有权限的人员确认归档。</p></div>
    <button className="button secondary" disabled={query.isFetching || history.isFetching} onClick={() => void refresh()}>重新核查</button></div>
    <ErrorNotice error={query.error} />{query.isPending && <Busy label="正在核对项目与财务事项" />}
    {p && <><p className={'notice ' + (p.visibility === 'restricted' ? 'neutral' : p.groups.some(g => (g.open_count ?? 0) > 0) ? 'warning' : 'success')}>
      {p.visibility === 'restricted' ? '部分依据尚未核实。权限受限不代表没有未结事项。' : p.groups.some(g => (g.open_count ?? 0) > 0) ? '仍有事项需要处置。请按下方清单查看原记录。' : '当前规则下未发现阻塞事项。请继续核对复盘与实际情况。'}</p>
      <div className="closeout-context"><span>项目版本 {p.project.version}</span><span>核查于 {new Intl.DateTimeFormat('zh-HK', { timeZone: 'Asia/Hong_Kong', dateStyle: 'medium', timeStyle: 'short' }).format(new Date(p.as_of))}</span></div>
      <Checklist groups={p.groups} projectID={project.id} />
      {!viewer && <div className="closeout-actions">{p.can_close && me.actions.includes('project.update') && <button className="button primary" onClick={() => setDialog('closing')}>进入结项核查阶段</button>}
        {p.can_archive && me.actions.includes('project.archive') && <button className="button primary" onClick={() => setDialog('archive')}>填写复盘并归档</button>}
        {p.project.state === 'closing' && <p className="muted">结项核查阶段的业务内容为只读。需处理事项时，请到项目概览恢复进行，处理后重新核查。</p>}</div>}
    </>}
    <section className="closeout-history"><h2>归档与复盘记录</h2><ErrorNotice error={history.error} />{history.isPending ? <Busy label="正在读取归档记录" /> : !history.error && history.data?.pages.some(page => page.visibility === 'restricted') ? <p className="notice neutral">当前权限无法完整查阅归档依据。历史记录、数量和复盘内容均未展示。</p> : !history.error && !archives.length ? <Empty title="尚无归档记录" detail="完成结项核查并提交复盘后，记录会保存在这里。" /> : archives.map(record => <ArchiveRecord key={record.id} record={record} projectID={project.id} />)}
      {history.hasNextPage && <button className="button secondary" disabled={history.isFetchingNextPage} onClick={() => void history.fetchNextPage()}>加载更早记录</button>}
    </section>
    {dialog === 'closing' && p && <CloseDialog preview={p} onClose={() => setDialog(null)} onSuccess={async () => { setDialog(null); await refresh(); }} />}
    {dialog === 'archive' && p && <ArchiveDialog initial={p} onClose={() => setDialog(null)} onSuccess={async () => { setDialog(null); await refresh(); }} />}
  </div>;
}
function ArchiveRecord({ record, projectID }: { record: Archive; projectID: string }) {
  return <details className="closeout-archive"><summary><strong>{record.project_name} · 归档版本 {record.project_version}</strong><span>{date(record.created_at)}</span></summary>
    <p className="muted">{record.current_basis_state === 'current' ? '当前项目仍与这次归档依据一致。' : '项目状态或核查依据已有变化。这份记录保留归档当时的情况，请查看当前核查清单。'}</p>
    <h3>成果与交付总结</h3><p className="closeout-prose">{record.summary}</p><h3>经验与复盘</h3><p className="closeout-prose">{record.lessons_learned}</p><h3>后续跟进</h3><p className="closeout-prose">{record.follow_up}</p><p>归档原因：{record.reason}</p>
    <p className="muted">归档人：{record.actor_name}。已明确确认业务闭环与合同、财务及订阅处置。记录于 {new Date(record.created_at).toLocaleString('zh-HK', { timeZone: 'Asia/Hong_Kong' })}。</p>
    {!!record.attachments.length && <ul className="closeout-files">{record.attachments.map(file => <li key={file.version_id}><span>{file.name} · 版本 {file.seq}</span><small>{file.size_bytes} B · SHA-256 {file.sha256}</small></li>)}</ul>}
    <Checklist groups={record.groups} projectID={projectID} historical />
  </details>;
}
function CloseDialog({ preview, onClose, onSuccess }: { preview: Preview; onClose: () => void; onSuccess: () => Promise<void> }) {
  const [reason, setReason] = useState(''); const key = useCommandKey();
  const mutation = useMutation({ mutationFn: () => { const body = { expected_version: preview.project.version, action: 'close', reason }; return api<Project>('/projects/' + preview.project.id + '/transition', { method: 'POST', body, key: key(body) }); }, onSuccess });
  return <Dialog title="进入结项核查阶段" onClose={onClose} busy={mutation.isPending}><form onSubmit={e => { e.preventDefault(); mutation.mutate(); }}><p className="notice neutral">提交后，普通项目业务内容将进入只读状态。系统会再次核对全部未结事项，随后仍需填写复盘并单独确认归档。</p><ErrorNotice error={mutation.error} /><Field label="核查原因"><input value={reason} onChange={e => setReason(e.target.value)} maxLength={200} required /></Field><div className="dialog-actions"><button type="button" className="button secondary" disabled={mutation.isPending} onClick={onClose}>返回核查</button><button className="button primary" disabled={mutation.isPending || !reason.trim()}>确认进入核查阶段</button></div></form></Dialog>;
}
function ArchiveDialog({ initial, onClose, onSuccess }: { initial: Preview; onClose: () => void; onSuccess: () => Promise<void> }) {
  const [basis, setBasis] = useState(initial), [summary, setSummary] = useState(''), [lessons, setLessons] = useState(''), [followUp, setFollowUp] = useState(''), [reason, setReason] = useState('');
  const [business, setBusiness] = useState(false), [financial, setFinancial] = useState(false), [fresh, setFresh] = useState(false), [picker, setPicker] = useState(false), [files, setFiles] = useState<FileReference[]>([]), [refreshing, setRefreshing] = useState(false), [refreshError, setRefreshError] = useState<unknown>();
  const key = useCommandKey(), errorRef = useRef<HTMLDivElement>(null);
  const mutation = useMutation({ mutationFn: () => { const body: Schemas['ProjectArchiveInput'] = { expected_version: basis.project.version, expected_fingerprint: basis.fingerprint!, summary, lessons_learned: lessons, follow_up: followUp, reason, business_verified: true, financial_verified: true, file_version_ids: files.map(file => file.version_id) }; return api<Schemas['ProjectArchiveReceipt']>('/projects/' + basis.project.id + '/archive', { method: 'POST', body, key: key(body) }); }, onSuccess });
  const changed = mutation.error instanceof ApiError && [403, 404, 409, 412].includes(mutation.error.status);
  useEffect(() => { if (mutation.error || refreshError) errorRef.current?.focus(); }, [mutation.error, refreshError]);
  async function reload() { setRefreshing(true); setRefreshError(undefined); try { const value = await api<Preview>('/projects/' + basis.project.id + '/closeout'); setBasis(value); setBusiness(false); setFinancial(false); mutation.reset(); } catch (error) { setRefreshError(error); } finally { setRefreshing(false); } }
  const ready = basis.can_archive && basis.visibility === 'complete' && !!basis.fingerprint;
  const error = refreshError ?? mutation.error;
  return <Dialog title="填写复盘并归档" onClose={onClose} busy={mutation.isPending || refreshing}><form className="project-archive-form" onSubmit={e => { e.preventDefault(); mutation.mutate(); }}>
    <p>将归档「{basis.project.name}」版本 {basis.project.version}。请核对全部事项并填写复盘。归档后，普通项目业务内容保持只读。</p>
    <div ref={errorRef} tabIndex={-1}><ErrorNotice error={error} />{error instanceof ApiError && !!error.details?.length && <ul>{error.details.filter(detail => ['summary', 'lessons_learned', 'follow_up', 'reason', 'attestations'].includes(detail.field ?? '')).map((detail, i) => <li key={i}><a href={'#archive-' + (detail.field === 'attestations' ? 'business' : detail.field)} onClick={e => { e.preventDefault(); document.getElementById('archive-' + (detail.field === 'attestations' ? 'business' : detail.field))?.focus(); }}>{detail.safe_message}</a></li>)}</ul>}</div>
    {(changed || !ready) && <p className="notice warning">当前依据或权限需要重新核对。已填写的复盘会保留，重新读取后请再次确认。</p>}
    <button type="button" className="button secondary" disabled={mutation.isPending || refreshing} onClick={() => void reload()}>{refreshing ? '正在重新核查' : '保留填写内容，重新读取核查'}</button>
    <Checklist groups={basis.groups} projectID={basis.project.id} />
    <Field label="成果与交付总结"><textarea id="archive-summary" value={summary} onChange={e => setSummary(e.target.value)} required maxLength={20000} rows={4} /></Field>
    <Field label="经验与复盘"><textarea id="archive-lessons_learned" value={lessons} onChange={e => setLessons(e.target.value)} required maxLength={20000} rows={4} /></Field>
    <Field label="后续跟进" hint="无遗留跟进事项时，请明确填写；此处不能代替未结事项的实际处置。"><textarea id="archive-follow_up" value={followUp} onChange={e => setFollowUp(e.target.value)} required maxLength={20000} rows={3} /></Field>
    <section aria-label="复盘附件"><h3>复盘附件（可选）</h3><ul className="closeout-files">{files.map(file => <li key={file.version_id}><div><span>{file.name} · 版本 {file.seq}</span><small>{file.size_bytes} B</small></div><button type="button" className="text-button" onClick={() => setFiles(items => items.filter(item => item.version_id !== file.version_id))} aria-label={'移除附件 ' + file.name}>移除</button></li>)}</ul><button type="button" className="button secondary" disabled={files.length >= 20 || mutation.isPending} onClick={() => setPicker(true)}>选择固定文件版本</button></section>
    <Field label="归档原因"><input id="archive-reason" value={reason} onChange={e => setReason(e.target.value)} maxLength={200} required /></Field>
    <label className="closeout-attestation"><input id="archive-business" type="checkbox" checked={business} onChange={e => setBusiness(e.target.checked)} />我已核对任务、里程碑、交付验收、风险、问题与变更，确认业务已闭环。</label>
    <label className="closeout-attestation"><input type="checkbox" checked={financial} onChange={e => setFinancial(e.target.checked)} />我已核对合同履约、采购、报销、实际费用、资金退款与订阅账期，确认当前没有待处置事项。</label>
    <MFAConfirmation onFresh={setFresh} />
    <p className="form-note">网络异常时，请保留此窗口并重试同一内容，或重新读取项目状态确认结果。</p>
    <div className="dialog-actions"><button type="button" className="button secondary" disabled={mutation.isPending || refreshing} onClick={onClose}>返回项目</button><button className="button primary" disabled={mutation.isPending || refreshing || changed || !ready || !business || !financial || !fresh || !summary.trim() || !lessons.trim() || !followUp.trim() || !reason.trim()}>{mutation.isPending ? '正在核对并归档' : '确认复盘并归档'}</button></div>
  </form>{picker && <FileVersionPicker project={basis.project.id} classification={basis.project.classification ?? 'internal'} title="选择复盘附件版本" onClose={() => setPicker(false)} onSelect={file => { setFiles(items => items.some(item => item.version_id === file.version_id) ? items : [...items, file]); setPicker(false); }} />}</Dialog>;
}
