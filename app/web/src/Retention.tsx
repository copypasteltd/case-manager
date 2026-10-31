import { useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowLeft, ClipboardCheck, ShieldCheck } from 'lucide-react';
import { api, ApiError, ensureDownloadSession, type Me, type Page, type Schemas } from './api';
import { Busy, date, Dialog, Empty, ErrorNotice, Field } from './components';
import { MFAConfirmation } from './MFAConfirmation';

type StoredFile = Schemas['File'];
type FileVersion = Schemas['FileVersion'];
type Overview = Schemas['RetentionOverview'];
type Hold = Schemas['RetentionHold'];
type Policy = Schemas['RetentionPolicy'];
type Assessment = Schemas['RetentionAssessment'];
const kinds = { legal: '法务保留', business: '业务保留', investigation: '调查保留' };
const policies: Record<Policy['kind'], string> = { formal_history: '正式历史版本', trash: '回收站文件', draft: '未引用草稿', multipart: '未完成分片' };
const referenceNames: Record<string, string> = {
  knowledge_revision: '知识固定修订', comment: '评论附件', approval_snapshot: '审批快照', contract_revision: '合同修订', contract_signature: '合同签署件',
  purchase_receipt: '采购收货依据', claim_revision: '报销修订', deliverable: '项目交付物', recovery_set: '有效恢复集',
};
const reasons: Record<string, string> = {
  POLICY_REQUIRED: '尚未发布对应保留制度', POLICY_DISABLED: '当前制度未启用评估', RETENTION_PERIOD_ACTIVE: '仍在保留期内',
  CURRENT_VERSION: '当前正式版本始终受保护', LEGAL_HOLD: '存在文件级法务保留标记', RETENTION_HOLD: '存在有效保留约束',
  BUSINESS_REFERENCE: '仍有业务或恢复集固定引用', RESTORE_ACTIVE: '历史恢复任务仍需读取此版本', UPLOAD_ACTIVE: '仍有上传会话以此版本为基础',
  DOWNLOAD_PERMIT_ACTIVE: '已签发的下载许可尚未过期', REPLICA_EVIDENCE_INCOMPLETE: '双云副本尚未具备完整验证证据',
  DERIVED_OBJECT_REVIEW_REQUIRED: '还需核对派生预览等对象的清理范围',
};
const moment = (value: string) => new Intl.DateTimeFormat('zh-HK', { timeZone: 'Asia/Hong_Kong', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value));
function useKey() {
  const last = useRef({ body: '', key: '' });
  return (body: unknown) => { const encoded = JSON.stringify(body); if (last.current.body !== encoded) last.current = { body: encoded, key: crypto.randomUUID() }; return last.current.key; };
}
export function useRetention(fileID: string, enabled: boolean) {
  return useQuery({ queryKey: ['retention', fileID], enabled, queryFn: ({ signal }) => api<Overview>('/files/' + fileID + '/retention', { signal }), refetchInterval: 30_000 });
}
async function refreshRetention(cache: ReturnType<typeof useQueryClient>, id: string) {
  await Promise.all([
    cache.invalidateQueries({ queryKey: ['retention', id] }), cache.invalidateQueries({ queryKey: ['retention-holds', id] }),
    cache.invalidateQueries({ queryKey: ['file', id] }), cache.invalidateQueries({ queryKey: ['files'] }),
  ]);
}
export function RetentionPanel({ file, overview, loading, error }: { file: StoredFile; overview?: Overview; loading: boolean; error: unknown }) {
  const [creating, setCreating] = useState(false);
  const canManage = !!overview?.can_manage && !error;
  return <section className="section-block" id="file-retention" aria-label="文件保留与引用">
    <div className="section-heading"><div><span className="eyebrow">KEEP WHAT MATTERS</span><h2>保留与引用保护</h2></div>
      {canManage && <button className="button secondary" onClick={() => setCreating(true)}><ShieldCheck size={16} />添加文件保留</button>}</div>
    <ErrorNotice error={error} />
    {loading ? <Busy label="正在读取保留约束" /> : !error && overview && <div className="panel retention-summary">
      <p>{overview.legal_hold ? '存在文件级法务保留标记。' : '未设置文件级法务保留标记。'}有效保留约束：{overview.active_hold_count.toLocaleString()} 项。</p>
      <p className="muted">移入回收站会保留文件历史与存储对象。有效保留约束会阻止移入回收站；解除某项约束会保留原记录与释放依据。</p>
      {canManage ? <Holds file={file} /> : <p className="muted">详细保留依据由获授权的文件保留管理人查阅和处理。</p>}
    </div>}
    {creating && canManage && <HoldEditor file={file} onClose={() => setCreating(false)} />}
  </section>;
}
function Holds({ file }: { file: StoredFile }) {
  const [releasing, setReleasing] = useState<Hold | null>(null);
  const query = useInfiniteQuery({ queryKey: ['retention-holds', file.id], initialPageParam: '',
    queryFn: ({ pageParam, signal }) => api<Page<Hold>>('/files/' + file.id + '/retention-holds?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }),
    getNextPageParam: page => page.next_cursor ?? undefined, refetchInterval: 30_000 });
  const entries = query.error ? [] : [...new Map(query.data?.pages.flatMap(page => page.items).map(item => [item.id, item])).values()];
  return <><ErrorNotice error={query.error} />{query.isPending ? <Busy label="正在读取保留记录" /> : !query.error && !entries.length ? <p className="muted">尚无保留记录。</p> : entries.map(hold =>
    <article className="retention-record" key={hold.id}><div className="section-heading"><h3>{kinds[hold.kind as keyof typeof kinds] ?? hold.kind}</h3><span className={'badge ' + (hold.active ? 'warning' : 'neutral')}>{hold.released_at ? '已释放' : hold.active ? '保留生效中' : '已过期'}</span></div>
      <p>{hold.reason}</p><dl className="file-facts"><div><dt>保护范围</dt><dd>{hold.version_id ? '一个固定内容版本' : '文件及全部内容版本'}</dd></div>
        <div><dt>有效期</dt><dd>{hold.valid_until ? moment(hold.valid_until) : '持续保留，直至明确释放'}</dd></div><div><dt>建立时间</dt><dd>{moment(hold.created_at)}</dd></div>
        {hold.released_at && <><div><dt>释放时间</dt><dd>{moment(hold.released_at)}</dd></div><div className="wide"><dt>释放依据</dt><dd>{hold.release_reason}</dd></div></>}</dl>
      {hold.active && <button className="text-button" onClick={() => setReleasing(hold)}>核对并释放此保留约束</button>}
    </article>)}
    {query.hasNextPage && <div className="list-footer"><span>已显示 {entries.length} 项</span><button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更多保留记录</button></div>}
    {releasing && <HoldRelease file={file} hold={releasing} onClose={() => setReleasing(null)} />}
  </>;
}
function HoldEditor({ file, version, onClose }: { file: StoredFile; version?: FileVersion; onClose: () => void }) {
  const [base] = useState(file), [kind, setKind] = useState<'legal' | 'business' | 'investigation'>('legal');
  const [reason, setReason] = useState(''), [until, setUntil] = useState(''), [fresh, setFresh] = useState(false);
  const key = useKey(), cache = useQueryClient();
  const mutation = useMutation({ mutationFn: async () => {
    await ensureDownloadSession();
    const body: Schemas['RetentionHoldCreate'] = { expected_version: base.version, kind, reason, ...(version ? { version_id: version.id } : {}), ...(until ? { valid_until: new Date(until).toISOString() } : {}) };
    return api<Hold>('/files/' + base.id + '/retention-holds', { method: 'POST', body, key: key(body) });
  }, onSuccess: async () => { await refreshRetention(cache, base.id); onClose(); } });
  const conflict = mutation.error instanceof ApiError && ['VERSION_CONFLICT', 'AUTHORIZATION_CHANGED'].includes(mutation.error.code);
  return <Dialog title={version ? '保留内容版本 ' + version.seq : '添加文件保留约束'} onClose={onClose} busy={mutation.isPending}>
    <form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}><p>{base.name} · {version ? '仅保护所选固定版本，并阻止文件移入回收站。' : '保护文件及其全部内容版本。'}</p>
      <ErrorNotice error={mutation.error} />{conflict && <p className="notice neutral">文件或授权已变化。请保留填写内容，读取最新资料后重新核对。</p>}
      <Field label="保留类型"><select value={kind} onChange={event => setKind(event.target.value as typeof kind)}>{Object.entries(kinds).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></Field>
      <Field label="保留依据"><textarea required rows={3} maxLength={200} value={reason} onChange={event => setReason(event.target.value)} /></Field>
      <Field label="保留至" hint="可留空表示持续保留；填写时间按当前设备时区解释。"><input type="datetime-local" value={until} onChange={event => setUntil(event.target.value)} /></Field>
      <MFAConfirmation onFresh={setFresh} /><div className="dialog-actions"><button type="button" className="button secondary" disabled={mutation.isPending} onClick={onClose}>取消</button><button className="button primary" disabled={mutation.isPending || !fresh || conflict}>确认建立保留约束</button></div>
    </form></Dialog>;
}
function HoldRelease({ file, hold, onClose }: { file: StoredFile; hold: Hold; onClose: () => void }) {
  const [reason, setReason] = useState(''), [fresh, setFresh] = useState(false);
  const cache = useQueryClient(), key = useKey();
  const mutation = useMutation({ mutationFn: async () => {
    await ensureDownloadSession(); const body = { reason };
    return api<Hold>('/retention-holds/' + hold.id + '/release', { method: 'POST', body, key: key(body) });
  }, onSuccess: async () => { await refreshRetention(cache, file.id); onClose(); } });
  const blocked = mutation.error instanceof ApiError && ['RETENTION_HOLD_RELEASED', 'AUTHORIZATION_CHANGED'].includes(mutation.error.code);
  return <Dialog title="释放此保留约束" onClose={onClose} busy={mutation.isPending}><form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}>
    <p><strong>{kinds[hold.kind as keyof typeof kinds] ?? hold.kind}</strong> · {file.name}</p><blockquote>{hold.reason}</blockquote>
    <p className="muted">释放会保留原约束与本次依据。文件上的其他保留约束、法务标记及业务引用仍各自生效。</p><ErrorNotice error={mutation.error} />
    <Field label="释放依据"><textarea required rows={3} maxLength={200} value={reason} onChange={event => setReason(event.target.value)} /></Field>
    <MFAConfirmation onFresh={setFresh} /><div className="dialog-actions"><button type="button" className="button secondary" disabled={mutation.isPending} onClick={onClose}>取消</button><button className="button primary" disabled={mutation.isPending || !fresh || blocked}>确认释放此约束</button></div>
  </form></Dialog>;
}
export function RetentionVersionActions({ file, version, canManage }: { file: StoredFile; version: FileVersion; canManage: boolean }) {
  const [mode, setMode] = useState<'hold' | 'assess' | null>(null);
  if (!canManage) return null;
  return <><button className="button secondary" onClick={() => setMode('assess')}><ClipboardCheck size={16} />保留评估</button>
    <button className="text-button" onClick={() => setMode('hold')}>保留此版本</button>
    {mode === 'hold' && <HoldEditor file={file} version={version} onClose={() => setMode(null)} />}
    {mode === 'assess' && <AssessmentDialog file={file} version={version} onClose={() => setMode(null)} />}</>;
}
function AssessmentDialog({ file, version, onClose }: { file: StoredFile; version: FileVersion; onClose: () => void }) {
  const [base] = useState(file), [reason, setReason] = useState(''); const key = useKey();
  const mutation = useMutation({ mutationFn: async () => {
    await ensureDownloadSession(); const body = { expected_file_version: base.version, reason };
    return api<Assessment>('/file-versions/' + version.id + '/retention-assessments', { method: 'POST', body, key: key(body) });
  } });
  const snapshot = mutation.data?.snapshot;
  return <Dialog title={'内容版本 ' + version.seq + ' 的保留评估'} onClose={onClose} busy={mutation.isPending}>
    <p>{base.name}</p><ErrorNotice error={mutation.error} />
    {snapshot ? <><div className="notice neutral">评估已留存，仅用于复核。清理已知原件请另行建立原件观察清单。</div>
      <dl className="file-facts"><div><dt>评估时间</dt><dd>{moment(snapshot.checked_at)}</dd></div><div><dt>评估结论</dt><dd>{snapshot.eligible_for_review ? '可进入后续人工复核' : '存在保护或证据缺口'}</dd></div>
        <div><dt>保留制度</dt><dd>{snapshot.policy ? policies[snapshot.policy.kind] + ' · 修订 ' + snapshot.policy.version : '尚未发布'}</dd></div><div><dt>保留期至</dt><dd>{snapshot.retain_until ? moment(snapshot.retain_until) : '尚无制度依据'}</dd></div>
        {base.state === 'trashed' && <div><dt>回收站附加制度</dt><dd>{snapshot.trash_policy ? '修订 ' + snapshot.trash_policy.version : '尚未发布，仍受保护'}</dd></div>}
        <div><dt>有效保留约束</dt><dd>{snapshot.active_hold_count} 项</dd></div><div><dt>未过期下载许可</dt><dd>{snapshot.unexpired_download_count} 项</dd></div>
        <div><dt>恢复与上传依赖</dt><dd>{snapshot.active_restore_count + snapshot.active_upload_count} 项</dd></div><div><dt>待核对派生对象</dt><dd>{snapshot.derived_object_count} 项</dd></div></dl>
      {!!snapshot.blocking_reasons.length && <div className="notice warning"><ul>{snapshot.blocking_reasons.map(code => <li key={code}>{reasons[code] ?? code}</li>)}</ul></div>}
      {!!Object.keys(snapshot.reference_counts).length && <><h3>固定业务引用</h3><dl className="file-facts">{Object.entries(snapshot.reference_counts).map(([kind, count]) => <div key={kind}><dt>{referenceNames[kind] ?? kind}</dt><dd>{count} 项</dd></div>)}</dl></>}
      <p className="muted">此记录固定保存评估时的事实。关闭后可重新评估最新状态。</p><div className="dialog-actions"><button className="button primary" onClick={onClose}>完成核对</button></div>
    </> : <form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}><p className="muted">核对保留制度、固定引用、下载许可和正在使用此版本的任务。评估会留存记录，供后续审阅。</p>
      <Field label="评估原因"><textarea required rows={3} maxLength={200} value={reason} onChange={event => setReason(event.target.value)} /></Field>
      <div className="dialog-actions"><button type="button" className="button secondary" disabled={mutation.isPending} onClick={onClose}>取消</button><button className="button primary" disabled={mutation.isPending || mutation.error instanceof ApiError && ['VERSION_CONFLICT', 'AUTHORIZATION_CHANGED'].includes(mutation.error.code)}>生成保留评估</button></div>
    </form>}
  </Dialog>;
}
export function RetentionPolicyPage({ me }: { me: Me }) {
  const [editing, setEditing] = useState<Policy['kind'] | null>(null);
  const query = useQuery({ queryKey: ['retention-policies'], enabled: me.actions.includes('config.manage'), queryFn: ({ signal }) => api<Page<Policy>>('/storage/retention-policies', { signal }) });
  if (!me.actions.includes('config.manage')) return <Empty title="当前账号没有制度管理权限" detail="需要明确的组织配置管理授权。" />;
  return <><Link className="back-link" to="/storage/scan-policies"><ArrowLeft size={16} />返回扫描例外制度</Link><div className="page-heading"><div><span className="eyebrow">KEEP WITH A REASON</span><h1>文件保留制度<span className="accent">.</span></h1><p>按用途分别发布保留时长与观察期，持续保存每次修订。</p></div></div>
    <p className="notice neutral">未发布或未启用的制度会阻断保留评估。发布制度不会执行删除；当前版本、保留约束及业务引用仍各自受保护。</p>
    <ErrorNotice error={query.error} />{query.isPending ? <Busy label="正在读取保留制度" /> : !query.error && <div className="policy-grid">{Object.entries(policies).map(([kind, label]) => {
      const policy = query.data?.items.find(item => item.kind === kind);
      return <article className="panel policy-card" key={kind}><div className="section-heading"><h2>{label}</h2><span className={'badge ' + (policy?.enabled ? 'success' : 'neutral')}>{policy?.enabled ? '纳入保留评估' : '尚未启用评估'}</span></div>
        {policy ? <><p className="muted">修订 {policy.version} · {date(policy.created_at)}</p><dl className="file-facts"><div><dt>保留时长</dt><dd>{(policy.retain_seconds / 86400).toLocaleString()} 天</dd></div><div><dt>观察期</dt><dd>{(policy.observation_seconds / 3600).toLocaleString()} 小时</dd></div></dl><p>{policy.reason}</p></> : <p className="muted">尚未发布此类制度；没有代填保留期限。</p>}
        <button className="button secondary" onClick={() => setEditing(kind as Policy['kind'])}>{policy ? '发布制度新修订' : '制定保留制度'}</button></article>;
    })}</div>}
    {editing && <RetentionPolicyEditor kind={editing} policy={query.data?.items.find(item => item.kind === editing)} onClose={() => setEditing(null)} />}
  </>;
}
function RetentionPolicyEditor({ kind, policy, onClose }: { kind: Policy['kind']; policy?: Policy; onClose: () => void }) {
  const [base] = useState(policy), [enabled, setEnabled] = useState(policy?.enabled ?? false), [days, setDays] = useState(policy ? String(policy.retain_seconds / 86400) : '');
  const [hours, setHours] = useState(policy ? String(policy.observation_seconds / 3600) : ''), [reason, setReason] = useState(''), [fresh, setFresh] = useState(false);
  const cache = useQueryClient(), key = useKey();
  const mutation = useMutation({ mutationFn: async () => {
    await ensureDownloadSession(); const body: Schemas['RetentionPolicyInput'] = {
      expected_version: base?.version ?? 0, kind, enabled, retain_seconds: Math.round(Number(days) * 86400), observation_seconds: Math.round(Number(hours) * 3600), reason,
    };
    return api<Policy>('/storage/retention-policies', { method: 'POST', body, key: key(body) });
  }, onSuccess: async () => { await Promise.all([cache.invalidateQueries({ queryKey: ['retention-policies'] }), cache.invalidateQueries({ queryKey: ['me'] })]); onClose(); } });
  const conflict = mutation.error instanceof ApiError && ['VERSION_CONFLICT', 'AUTHORIZATION_CHANGED'].includes(mutation.error.code);
  return <Dialog title={'发布' + policies[kind] + '保留制度'} onClose={onClose} busy={mutation.isPending}><form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}>
    <ErrorNotice error={mutation.error} />{conflict && <p className="notice neutral">制度或授权已变化。请保留填写内容，读取最新修订后重新核对。</p>}
    <label className="checkbox-row"><input type="checkbox" checked={enabled} onChange={event => setEnabled(event.target.checked)} />启用此类保留评估</label>
    <Field label="保留时长（天）" hint="须明确填写，至少1天。业务引用和法务保留可能要求继续保留。"><input required type="number" min="1" max="3650" step="any" value={days} onChange={event => setDays(event.target.value)} /></Field>
    <Field label="观察期（小时）" hint="须明确填写，至少24小时；观察期结束仍需重新核对依赖。"><input required type="number" min="24" max="8760" step="any" value={hours} onChange={event => setHours(event.target.value)} /></Field>
    <Field label="发布依据"><textarea required rows={3} maxLength={200} value={reason} onChange={event => setReason(event.target.value)} /></Field>
    <MFAConfirmation onFresh={setFresh} /><div className="dialog-actions"><button type="button" className="button secondary" disabled={mutation.isPending} onClick={onClose}>取消</button><button className="button primary" disabled={mutation.isPending || !fresh || conflict}>发布保留制度</button></div>
  </form></Dialog>;
}
