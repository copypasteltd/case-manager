import { useRef, useState } from 'react';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { History, ShieldCheck } from 'lucide-react';
import { api, ApiError, ensureDownloadSession, type Me, type Page, type Schemas } from './api';
import { Busy, classificationNames, date, Dialog, Empty, ErrorNotice, Field } from './components';
import { MFAConfirmation } from './MFAConfirmation';

type Policy = Schemas['ClassificationPolicy'];
const channels = { web: 'Web', wechat: '微信小程序', douyin: '抖音小程序', android: 'Android', telegram: 'Telegram' };
const editableChannels = ['web', 'wechat', 'douyin', 'android'] as const;
const levels = ['internal', 'project_restricted', 'confidential'] as const;

export default function ClassificationPolicies({ me }: { me: Me }) {
  const [editing, setEditing] = useState<Policy | null>(null), [history, setHistory] = useState<Policy['level'] | null>(null);
  const query = useQuery({ queryKey: ['classification-policies'], queryFn: ({ signal }) => api<Page<Policy>>('/classification-policies', { signal }), enabled: me.actions.includes('policy.manage') });
  if (!me.actions.includes('policy.manage')) return <Empty title="没有分类策略管理权限" detail="请联系负责人核对当前管理授权。" />;
  return <><div className="page-heading"><div><div className="eyebrow">WORKSPACE / DATA BOUNDARIES</div><h1>资料分类与渠道</h1><p>明确每类资料可以从哪些入口访问，以及是否允许秘书处理。</p></div><ShieldCheck size={24} /></div><div className="notice neutral"><ShieldCheck size={18} /><span>分类策略与业务权限共同限制访问。允许渠道或秘书处理，不会额外授予成员阅读、修改或审批权限。</span></div><ErrorNotice error={query.error} />{query.isPending ? <Busy label="正在读取当前分类策略" /> : !query.error && <div className="classification-policies">{levels.map(level => {
    const p = query.data?.items.find(item => item.level === level);
    return <section className="panel classification-policy" key={level}><div className="section-heading"><h2>{classificationNames[level]}</h2>{p && <span className="badge neutral">第 {p.version} 版</span>}</div>{p ? <><dl className="detail-list"><div><dt>秘书处理</dt><dd>{p.ai_allowed ? '允许，仍需满足全部权限与任务范围' : '未允许'}</dd></div><div><dt>访问入口</dt><dd>{p.allowed_channels.map(key => channels[key]).join('、') || '全部关闭'}</dd></div><div><dt>发布时间</dt><dd>{date(p.created_at)}</dd></div><div><dt>发布原因</dt><dd className="approval-reason">{p.reason ?? '原有策略未记录发布原因。'}</dd></div></dl><div className="post-actions"><button className="button primary" onClick={() => setEditing(p)}>发布策略新修订</button><button className="button secondary" onClick={() => setHistory(level)}><History size={16} />查看策略历史</button></div></> : <p className="muted">此分类尚无有效策略，访问保持默认拒绝。</p>}{level === 'confidential' && <p className="form-note">严格受限资料当前保持秘书处理关闭；需具备准确对象授权后通过员工入口查阅。</p>}</section>;
  })}</div>}{editing && <PolicyEditor policy={editing} onClose={() => setEditing(null)} />}{history && <PolicyHistory level={history} onClose={() => setHistory(null)} />}</>;
}

function PolicyEditor({ policy, onClose }: { policy: Policy; onClose: () => void }) {
  const [base] = useState(policy), [ai, setAI] = useState(policy.ai_allowed), [allowed, setAllowed] = useState(policy.allowed_channels), [reason, setReason] = useState(''), [fresh, setFresh] = useState(false);
  const cache = useQueryClient(), command = useRef({ body: '', key: '' });
  const mutation = useMutation({ mutationFn: async () => {
    await ensureDownloadSession();
    const body: Schemas['ClassificationPolicyInput'] = { level: base.level, base_policy_id: base.id, expected_version: base.version, ai_allowed: ai, allowed_channels: editableChannels.filter(key => allowed.includes(key)), reason };
    const text = JSON.stringify(body); if (text !== command.current.body) command.current = { body: text, key: crypto.randomUUID() };
    return api<Policy>('/classification-policies', { method: 'POST', body, key: command.current.key });
  }, onSuccess: async () => { await cache.invalidateQueries(); onClose(); } });
  const conflict = mutation.error instanceof ApiError && ['VERSION_CONFLICT', 'AUTHORIZATION_CHANGED'].includes(mutation.error.code);
  const unchanged = ai === base.ai_allowed && [...allowed].sort().join('|') === [...base.allowed_channels].sort().join('|');
  return <Dialog title={'发布' + classificationNames[base.level] + '策略'} onClose={onClose} busy={mutation.isPending}><form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}><ErrorNotice error={mutation.error} />{conflict && <p className="notice neutral">策略或授权已变化。已保留当前输入，请关闭后核对最新修订再发布。</p>}<p className="muted">本次修改影响此分类下的全部资料。发布后立即按新规则检查人工访问与秘书读取。</p><label className="checkbox-row"><input type="checkbox" checked={ai} disabled={base.level === 'confidential'} onChange={event => setAI(event.target.checked)} />允许秘书处理此分类下有权读取的资料</label>{base.level === 'confidential' && <p className="form-note">严格受限资料尚未开放秘书处理。</p>}<fieldset className="policy-reasons"><legend>允许的员工访问入口</legend>{editableChannels.map(key => <label className="checkbox-row" key={key}><input type="checkbox" checked={allowed.includes(key)} disabled={base.level === 'internal' && key === 'web'} onChange={event => setAllowed(value => event.target.checked ? [...value, key] : value.filter(item => item !== key))} />{channels[key]}</label>)}</fieldset><p className="form-note">公司内部策略保留 Web 管理入口。Telegram 群披露需独立验证，当前不在此处开放。</p><Field label="发布原因"><textarea required maxLength={200} rows={3} value={reason} onChange={event => setReason(event.target.value)} /></Field><MFAConfirmation onFresh={setFresh} /><div className="dialog-actions"><button type="button" className="button secondary" disabled={mutation.isPending} onClick={onClose}>取消</button><button className="button primary" disabled={mutation.isPending || !fresh || unchanged || !reason.trim() || conflict}>确认发布策略</button></div></form></Dialog>;
}

function PolicyHistory({ level, onClose }: { level: Policy['level']; onClose: () => void }) {
  const query = useInfiniteQuery({ queryKey: ['classification-policy-history', level], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Policy>>('/classification-policies/' + level + '/history?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const entries = query.error ? [] : query.data?.pages.flatMap(page => page.items) ?? [];
  return <Dialog title={classificationNames[level] + '策略历史'} onClose={onClose}><ErrorNotice error={query.error} />{query.isPending ? <Busy label="正在读取策略历史" /> : entries.map(p => <section className="classification-history" key={p.id}><h3>第 {p.version} 版</h3><p className="muted">发布于 {date(p.created_at)}</p><dl className="detail-list"><div><dt>秘书处理</dt><dd>{p.ai_allowed ? '允许' : '未允许'}</dd></div><div><dt>访问入口</dt><dd>{p.allowed_channels.map(key => channels[key]).join('、') || '全部关闭'}</dd></div><div><dt>发布原因</dt><dd className="approval-reason">{p.reason ?? '原有策略未记录发布原因。'}</dd></div></dl></section>)}{query.hasNextPage && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更早策略</button>}<div className="dialog-actions"><button className="button secondary" onClick={onClose}>关闭历史</button></div></Dialog>;
}
