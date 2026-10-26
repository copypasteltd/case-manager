import { useRef, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowLeft, ArrowUpRight, ShieldCheck } from 'lucide-react';
import { api, ApiError, ensureDownloadSession, type Me, type Page, type Schemas } from './api';
import { Busy, classificationNames, date, Dialog, Empty, ErrorNotice, Field } from './components';
import { bytes } from './UploadQueue';
import { MFAConfirmation } from './MFAConfirmation';

type Waiver = Schemas['ScanWaiver'];
type Policy = Schemas['ScanWaiverPolicy'];
const states: Record<string, string> = { pending: '待独立审核', approved: '已例外放行', rejected: '已拒绝', cancelled: '已取消' };
const codeLabels: Record<string, string> = { SCANNER_NOT_CONFIGURED: '扫描引擎未配置', SCAN_SIZE_LIMIT: '超过送检大小限制', SCAN_ENGINE_SIZE_LIMIT: '超过引擎大小限制', SCAN_CONTENT_LIMIT: '超过引擎内容处理限制' };
function useKey() {
  const last = useRef({ body: '', key: '' });
  return (body: unknown) => { const content = JSON.stringify(body); if (content !== last.current.body) last.current = { body: content, key: crypto.randomUUID() }; return last.current.key; };
}

export function ScanWaiversPage({ me }: { me: Me }) {
  const [state, setState] = useState('pending');
  const query = useInfiniteQuery({ queryKey: ['scan-waivers', me.user.id, state], initialPageParam: '',
    queryFn: ({ pageParam, signal }) => api<Page<Waiver>>('/scan-waivers?' + new URLSearchParams({ state, limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }),
    getNextPageParam: page => page.next_cursor ?? undefined, refetchInterval: 10_000 });
  const items = [...new Map(query.data?.pages.flatMap(page => page.items).map(item => [item.id, item])).values()];
  return <><Link className="back-link" to="/files"><ArrowLeft size={16} />返回文件中心</Link><div className="page-heading"><div><span className="eyebrow">REVIEW WITH EVIDENCE</span><h1>扫描例外申请<span className="accent">.</span></h1><p>只显示本人申请或由制度指定、且仍有范围权限的审核记录。</p></div>{me.actions.includes('config.manage') && <Link className="button secondary" to="/storage/scan-policies"><ShieldCheck size={17} />管理例外制度</Link>}</div><div className="notice warning">例外放行不代表扫描通过。无法扫描的固定源必须符合明确制度，并由独立审核人确认；恶意内容不能在此放行。</div><div className="toolbar"><label className="filter-label">申请状态 <select value={state} onChange={event => setState(event.target.value)}><option value="pending">待独立审核</option><option value="">全部记录</option><option value="approved">已例外放行</option><option value="rejected">已拒绝</option><option value="cancelled">已取消</option></select></label><button className="text-button" onClick={() => void query.refetch()}>刷新申请</button></div><ErrorNotice error={query.error} />{query.isPending ? <Busy label="正在读取例外申请" /> : !items.length ? <div className="panel"><Empty title="当前筛选下没有可见申请" detail="上传遇到无法扫描的情况时，可从上传队列查看报告并申请例外。" /></div> : <div className="panel file-list">{items.map(item => <Link className="file-row" to={'/scan-waivers/' + item.id} key={item.id}><ShieldCheck size={22} /><div><strong>{item.name}</strong><small>{item.owner_id === me.user.id ? '我的申请' : '待我审核'} · {bytes(item.size_bytes)} · {date(item.updated_at)}</small></div><span className={'badge ' + (item.state === 'approved' ? 'warning' : 'neutral')}>{states[item.state]}</span><ArrowUpRight size={17} /></Link>)}</div>}{query.hasNextPage && <div className="list-footer"><span>已显示 {items.length} 项</span><button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更多</button></div>}</>;
}

export function ScanWaiverDetail({ me }: { me: Me }) {
  const { id = '' } = useParams(), [action, setAction] = useState<'approve' | 'reject' | 'cancel' | null>(null);
  const query = useQuery({ queryKey: ['scan-waiver', id], queryFn: ({ signal }) => api<Waiver>('/scan-waivers/' + id, { signal }), refetchInterval: 10_000 });
  const report = useQuery({ queryKey: ['waiver-report', id], queryFn: ({ signal }) => api<Schemas['UploadScanReport']>('/scan-waivers/' + id + '/scan-report', { signal }), enabled: !!query.data });
  if (query.isPending) return <Busy label="正在读取固定例外申请" />;
  if (!query.data) return <><Link className="back-link" to="/scan-waivers"><ArrowLeft size={16} />返回申请列表</Link><ErrorNotice error={query.error} /></>;
  const value = query.data, pending = value.state === 'pending' && Date.parse(value.expires_at) > Date.now();
  const validReport = report.data?.id === value.scan_report_id && report.data.sha256 === value.sha256 && report.data.upload_id === value.upload_id && report.data.size_bytes === value.size_bytes;
  return <><Link className="back-link" to="/scan-waivers"><ArrowLeft size={16} />返回申请列表</Link><ErrorNotice error={query.error} /><div className="page-heading"><div><span className="eyebrow">A FIXED SOURCE, A CLEAR DECISION</span><h1>{value.name}</h1><p>{classificationNames[value.classification]} · {bytes(value.size_bytes)} · 制度修订 {value.policy_version}</p></div><span className={'badge ' + (value.state === 'approved' ? 'warning' : 'neutral')}>{states[value.state]}</span></div><div className="notice warning">申请对应下方固定摘要与不可变扫描报告。例外只改变该内容的放行状态，报告仍保留“无法扫描”的原始结论。</div><section className="panel"><h2>申请与内容证据</h2><dl className="file-facts"><div><dt>媒体类型</dt><dd>{value.media_type}</dd></div><div><dt>无法扫描原因</dt><dd>{codeLabels[value.scan_code] ?? value.scan_code}</dd></div><div><dt>申请原因</dt><dd>{value.reason}</dd></div><div><dt>有效至</dt><dd>{new Date(value.expires_at).toLocaleString('zh-HK', { timeZone: 'Asia/Hong_Kong' })}</dd></div><div className="wide"><dt>固定源 SHA-256</dt><dd><code>{value.sha256}</code></dd></div>{value.decision_reason && <div className="wide"><dt>处理原因</dt><dd>{value.decision_reason}</dd></div>}</dl><ErrorNotice error={report.error} />{report.isPending ? <Busy label="正在读取申请固定的扫描报告" /> : report.data && <div className="scan-evidence"><h3>原始扫描证据</h3><p>{report.data.state === 'unscannable' ? '无法扫描' : report.data.state === 'clean' ? '扫描通过' : '已阻断'} · 实际送检 {bytes(report.data.streamed_bytes)} / {bytes(report.data.size_bytes)}</p><p className="muted">引擎：{report.data.engine ?? '未提供'} · {report.data.engine_version ?? '未提供'}；签名库：{report.data.signature_revision ?? '未提供'}</p>{!validReport && <p className="notice danger">当前扫描证据与申请不一致，请联系管理员核对。</p>}</div>}</section>
    {pending && <div className="page-actions">{value.owner_id === me.user.id ? <button className="button secondary" onClick={() => setAction('cancel')}>取消我的申请</button> : value.reviewer_id === me.user.id && me.actions.includes('file.scan.waive') && <><button className="button secondary" onClick={() => setAction('reject')}>拒绝例外</button><button className="button primary" disabled={!validReport || report.data?.state !== 'unscannable'} onClick={() => setAction('approve')}>核对并决定放行</button></>}</div>}{value.state === 'pending' && !pending && <p className="notice neutral">申请已过有效期，不能再审核放行。</p>}{action && <WaiverDecision waiver={value} action={action} onClose={() => setAction(null)} />}</>;
}

function WaiverDecision({ waiver: original, action, onClose }: { waiver: Waiver; action: 'approve' | 'reject' | 'cancel'; onClose: () => void }) {
  const [waiver] = useState(original), [reason, setReason] = useState(''), [risk, setRisk] = useState(false), [fresh, setFresh] = useState(false);
  const key = useKey(), cache = useQueryClient();
  const mutation = useMutation({
    mutationFn: async () => {
      await ensureDownloadSession();
      const body = action === 'cancel' ? { expected_version: waiver.version, action: 'cancel', reason } : {
        expected_version: waiver.version, upload_version: waiver.upload_version, scan_report_id: waiver.scan_report_id, sha256: waiver.sha256,
        decision: action, reason, acknowledged_risk: risk,
      };
      return api('/scan-waivers/' + waiver.id + (action === 'cancel' ? '/cancel' : '/decisions'), { method: 'POST', body, key: key(body) });
    },
    onSuccess: async () => { await Promise.all([cache.invalidateQueries({ queryKey: ['scan-waiver', waiver.id] }), cache.invalidateQueries({ queryKey: ['scan-waivers'] }), cache.invalidateQueries({ queryKey: ['uploads'] })]); onClose(); },
  });
  const title = action === 'approve' ? '确认扫描例外放行' : action === 'reject' ? '拒绝扫描例外' : '取消我的例外申请';
  const conflict = mutation.error instanceof ApiError && ['VERSION_CONFLICT', 'SCAN_WAIVER_STALE', 'UPLOAD_STATE_CONFLICT', 'SCAN_WAIVER_POLICY_CHANGED'].includes(mutation.error.code);
  return <Dialog title={title} onClose={onClose} busy={mutation.isPending}><form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}><ErrorNotice error={mutation.error} />{conflict && <p className="notice neutral">申请、扫描或制度已经改变。请关闭窗口重新读取，当前填写内容保留供核对。</p>}<p><strong>{waiver.name}</strong> · {bytes(waiver.size_bytes)}</p><p className="digest-line">SHA-256：<code>{waiver.sha256}</code></p>{action === 'approve' && <><p className="notice warning">此决定会放行未经完整安全扫描的固定源，并继续服务端整理流程。正式内容将保留“经授权放行”标记。</p><label className="checkbox-row"><input type="checkbox" required checked={risk} onChange={event => setRisk(event.target.checked)} />我已核对报告、摘要、范围及制度，并接受此次放行风险</label></>}<Field label="处理原因"><textarea rows={3} required maxLength={200} value={reason} onChange={event => setReason(event.target.value)} /></Field>{action !== 'cancel' && <MFAConfirmation onFresh={setFresh} />}<div className="dialog-actions"><button className="button secondary" type="button" onClick={onClose} disabled={mutation.isPending}>暂不处理</button><button className="button primary" disabled={mutation.isPending || conflict || (action !== 'cancel' && !fresh) || (action === 'approve' && !risk)}>{title}</button></div></form></Dialog>;
}

export function ScanPolicyPage({ me }: { me: Me }) {
  const [editing, setEditing] = useState<'internal' | 'project_restricted' | null>(null);
  const query = useQuery({ queryKey: ['scan-policies'], queryFn: ({ signal }) => api<Page<Policy>>('/storage/scan-waiver-policies', { signal }), enabled: me.actions.includes('config.manage') });
  if (!me.actions.includes('config.manage')) return <Empty title="当前账号没有制度管理权限" detail="需要明确的组织配置管理授权。" />;
  return <><Link className="back-link" to="/scan-waivers"><ArrowLeft size={16} />返回例外申请</Link><div className="page-heading"><div><span className="eyebrow">EXPLICIT RULES, RECORDED REVISIONS</span><h1>扫描例外制度<span className="accent">.</span></h1><p>新修订会改变后续放行依据，历史修订持续保留。</p></div><Link className="button secondary" to="/storage/retention-policies">文件保留制度</Link></div><div className="notice warning">默认不允许扫描例外。严格受限内容和恶意内容无法通过此制度放行。审核人还必须具备对应资料范围的独立审核权限。</div><ErrorNotice error={query.error} />{query.isPending ? <Busy label="正在读取当前制度" /> : !query.error && <div className="policy-grid">{(['internal', 'project_restricted'] as const).map(classification => {
    const policy = query.data?.items.find(item => item.classification === classification);
    return <article className="panel policy-card" key={classification}><div className="section-heading"><h2>{classificationNames[classification]}</h2><span className={'badge ' + (policy?.enabled ? 'warning' : 'neutral')}>{policy?.enabled ? '例外已启用' : '例外未启用'}</span></div>{policy ? <><p className="muted">修订 {policy.version} · {date(policy.created_at)}</p><dl className="file-facts"><div><dt>允许的媒体类型</dt><dd>{policy.media_types.join('、')}</dd></div><div><dt>单文件上限</dt><dd>{bytes(policy.max_bytes)}</dd></div><div className="wide"><dt>允许的扫描原因</dt><dd>{policy.allowed_codes.map(code => codeLabels[code]).join('、')}</dd></div></dl></> : <p className="muted">尚未发布此分类的例外制度。</p>}<button className="button secondary" onClick={() => setEditing(classification)}>{policy ? '发布制度新修订' : '制定例外范围'}</button></article>;
  })}</div>}{editing && <PolicyEditor classification={editing} policy={query.data?.items.find(item => item.classification === editing)} onClose={() => setEditing(null)} />}</>;
}

function PolicyEditor({ classification, policy, onClose }: { classification: 'internal' | 'project_restricted'; policy?: Policy; onClose: () => void }) {
  const [base] = useState(policy), [enabled, setEnabled] = useState(policy?.enabled ?? false), [media, setMedia] = useState(policy?.media_types.join('\n') ?? ''),
    [codes, setCodes] = useState(policy?.allowed_codes ?? []), [max, setMax] = useState(policy?.max_bytes ?? ''), [reviewer, setReviewer] = useState(policy?.reviewer_id ?? ''),
    [reason, setReason] = useState(''), [fresh, setFresh] = useState(false);
  const cache = useQueryClient(), key = useKey();
  const users = useInfiniteQuery({ queryKey: ['policy-reviewer-users'], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Schemas['User']>>('/users?' + new URLSearchParams({ limit: '100', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const people = [...new Map(users.data?.pages.flatMap(page => page.items).filter(user => user.status === 'active').map(user => [user.id, user])).values()];
  const mutation = useMutation({ mutationFn: async () => {
    await ensureDownloadSession();
    const body: Schemas['ScanWaiverPolicyInput'] = { expected_version: base?.version ?? 0, classification, enabled, media_types: media.split(/\r?\n/).map(value => value.trim()).filter(Boolean), allowed_codes: codes, max_bytes: max, reviewer_id: reviewer, reason };
    return api('/storage/scan-waiver-policies', { method: 'POST', body, key: key(body) });
  }, onSuccess: async () => { await Promise.all([cache.invalidateQueries({ queryKey: ['scan-policies'] }), cache.invalidateQueries({ queryKey: ['me'] })]); onClose(); } });
  const conflict = mutation.error instanceof ApiError && mutation.error.code === 'VERSION_CONFLICT';
  return <Dialog title={'发布' + classificationNames[classification] + '扫描例外制度'} busy={mutation.isPending} onClose={onClose}><form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}><ErrorNotice error={mutation.error ?? users.error} />{conflict && <p className="notice neutral">制度已被修改，请关闭窗口读取最新修订后核对。</p>}<label className="checkbox-row"><input type="checkbox" checked={enabled} onChange={event => setEnabled(event.target.checked)} />启用此分类的明确扫描例外</label><Field label="允许的媒体类型" hint="每行一个精确 MIME 类型，例如 application/pdf；不接受通配或可执行类型。"><textarea required rows={3} value={media} onChange={event => setMedia(event.target.value)} /></Field><fieldset className="policy-reasons"><legend>允许的无法扫描原因</legend>{Object.entries(codeLabels).map(([code, label]) => <label className="checkbox-row" key={code}><input type="checkbox" checked={codes.includes(code as typeof codes[number])} onChange={event => setCodes(value => event.target.checked ? [...value, code as typeof codes[number]] : value.filter(item => item !== code))} />{label}</label>)}</fieldset><Field label="单文件最大允许字节数" hint="必须明确填写，范围为 1 至 120,000,000,000 字节。"><input required type="text" inputMode="numeric" pattern="[0-9]+" value={max} onChange={event => setMax(event.target.value)} /></Field><Field label="制度指定审核人" hint="选人不会自动授予内容访问或审核权限；仍需在成员与授权中配置对应范围。"><select required value={reviewer} onChange={event => setReviewer(event.target.value)}><option value="">请选择审核人</option>{people.map(user => <option key={user.id} value={user.id}>{user.display_name}</option>)}</select></Field>{users.hasNextPage && <button className="text-button" type="button" disabled={users.isFetchingNextPage} onClick={() => void users.fetchNextPage()}>读取更多人员</button>}<Field label="发布原因"><textarea rows={2} required maxLength={200} value={reason} onChange={event => setReason(event.target.value)} /></Field><MFAConfirmation onFresh={setFresh} /><div className="dialog-actions"><button type="button" className="button secondary" disabled={mutation.isPending} onClick={onClose}>取消</button><button className="button primary" disabled={mutation.isPending || !fresh || !codes.length || conflict}>发布新修订</button></div></form></Dialog>;
}
