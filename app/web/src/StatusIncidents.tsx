import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { useInfiniteQuery, useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowLeft, ArrowUpRight, Plus } from 'lucide-react';
import { api, type Me, type Page, type Schemas } from './api';
import { Busy, Dialog, Empty, ErrorNotice, Field } from './components';
import { MFAConfirmation } from './MFAConfirmation';
import { isStatusConflict, StatusConflict, StatusNavigation, useStatusKey } from './StatusShared';
import { statusTime } from './statusFreshness';

type Incident = Schemas['Incident'];
type Service = Schemas['Service'];
const states: Record<Incident['state'], string> = { open: '待处置', investigating: '调查中', monitoring: '观察恢复', resolved: '已解决' };
const severities: Record<Incident['severity'], string> = { low: '低', medium: '中', high: '高', critical: '严重' };
function IncidentState({ value }: { value: Incident['state'] }) { return <span className={'badge ' + (value === 'resolved' ? 'neutral' : value === 'monitoring' ? 'info' : 'warning')}>{states[value]}</span>; }
export function IncidentsPage({ me }: { me: Me }) {
  const [state, setState] = useState(''), [create, setCreate] = useState(false);
  const query = useInfiniteQuery({ queryKey: ['incidents', state], initialPageParam: '', enabled: me.actions.includes('service.read'), queryFn: ({ pageParam, signal }) => api<Page<Incident>>('/incidents?' + new URLSearchParams({ state, limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const items = query.error ? [] : [...new Map(query.data?.pages.flatMap(page => page.items).map(item => [item.id, item])).values()];
  if (!me.actions.includes('service.read')) return <Empty title="没有服务事件权限" detail="事件按全部影响服务的当前阅读范围展示。" />;
  return <><div className="page-heading"><div><span className="eyebrow">OPERATIONS / INCIDENTS</span><h1>事件记录</h1><p>明确影响范围，持续记录调查、恢复和复盘。</p></div>{me.actions.includes('incident.update') && <button className="button primary" onClick={() => setCreate(true)}><Plus size={17} />登记事件</button>}</div><StatusNavigation />
    <div className="party-filters"><Field label="事件进度"><select value={state} onChange={event => setState(event.target.value)}><option value="">全部进度</option>{Object.entries(states).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></Field><button className="button secondary" disabled={query.isFetching} onClick={() => void query.refetch()}>刷新记录</button></div>
    <ErrorNotice error={query.error} />{query.isPending ? <Busy label="正在读取事件" /> : !items.length && !query.error ? <Empty title="尚无符合条件的事件" detail="这里只展示全部影响服务均在你当前阅读范围内的事件。" /> : !!items.length && <div className="panel party-list">{items.map(item => <Link className="party-row" key={item.id} to={'/incidents/' + item.id}><div><h2>{item.title}</h2><p>{severities[item.severity]}等级 · 影响 {item.service_ids.length} 项服务 · {statusTime(item.updated_at)}（香港）</p></div><IncidentState value={item.state} /><ArrowUpRight size={18} /></Link>)}</div>}{query.hasNextPage && !query.error && <div className="list-footer"><span>已显示 {items.length} 项事件</span><button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更多事件</button></div>}{create && <IncidentCreate me={me} onClose={() => setCreate(false)} />}
  </>;
}
export function IncidentDetail() {
  const { id = '' } = useParams(), [edit, setEdit] = useState(false);
  const query = useQuery({ queryKey: ['incident', id], queryFn: ({ signal }) => api<Incident>('/incidents/' + id, { signal }) });
  const value = query.error ? undefined : query.data;
  if (!value) return <><Link className="back-link" to="/incidents"><ArrowLeft size={16} />事件记录</Link><ErrorNotice error={query.error} />{query.isPending && <Busy />}</>;
  return <><Link className="back-link" to="/incidents"><ArrowLeft size={16} />事件记录</Link><div className="page-heading"><div><span className="eyebrow">INCIDENT / RESPONSE LOG</span><h1>{value.title}</h1><p>{severities[value.severity]}等级 · 当前第 {value.version} 版</p></div><div className="heading-actions"><IncidentState value={value.state} /><button className="button secondary" disabled={query.isFetching} onClick={() => void query.refetch()}>刷新记录</button>{value.can_update && <button className="button primary" onClick={() => setEdit(true)}>{value.state === 'resolved' ? '追加复盘' : '更新处置进展'}</button>}</div></div>
    <section className="panel party-detail"><h2>事件说明</h2><p className="status-preserve">{value.description || '尚未补充说明'}</p><h3>影响服务</h3><ul className="status-impact-list">{value.service_ids.map(serviceID => <li key={serviceID}><AffectedService id={serviceID} /></li>)}</ul>{value.resolution && <><h3>恢复说明</h3><p className="status-preserve">{value.resolution}</p></>}{value.resolved_at && <p className="muted">首次解决于 {statusTime(value.resolved_at)}（香港时间）</p>}{value.postmortem && <><h3>复盘</h3><p className="status-preserve">{value.postmortem}</p></>}</section>
    <section className="section-block status-section"><div className="section-heading"><h2>处置时间线</h2><span className="muted">新记录在前；原记录持续保留</span></div><IncidentTimeline id={id} /></section>{edit && <IncidentUpdate incident={value} onClose={() => setEdit(false)} />}
  </>;
}
function AffectedService({ id }: { id: string }) {
  const query = useQuery({ queryKey: ['incident-service-label', id], queryFn: ({ signal }) => api<Service>('/services/' + id, { signal }) });
  return <><ErrorNotice error={query.error} />{!query.error && <Link className="text-button" to={'/services/' + id}>{query.data?.name ?? '查看影响服务'} <ArrowUpRight size={14} /></Link>}</>;
}
function IncidentTimeline({ id }: { id: string }) {
  const query = useInfiniteQuery({ queryKey: ['incident-timeline', id], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Schemas['IncidentEntry']>>('/incidents/' + id + '/timeline?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const items = query.error ? [] : query.data?.pages.flatMap(page => page.items) ?? [];
  return <><ErrorNotice error={query.error} />{query.isPending ? <Busy /> : !items.length && !query.error ? <Empty title="尚无时间线记录" detail="事件建立和后续处置均应追加到时间线。" /> : items.map(item => <article className="panel status-history-entry" key={item.id}><div className="section-heading compact"><h3>第 {item.seq} 条 · {statusTime(item.created_at)}（香港）</h3><IncidentState value={item.state} /></div><p className="status-preserve">{item.note}</p>{item.resolution && <p className="status-preserve">恢复说明：{item.resolution}</p>}{item.postmortem && <p className="status-preserve">复盘：{item.postmortem}</p>}</article>)}{query.hasNextPage && !query.error && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更早时间线</button>}</>;
}
function IncidentCreate({ me, onClose }: { me: Me; onClose: () => void }) {
  const [title, setTitle] = useState(''), [severity, setSeverity] = useState<Incident['severity']>('medium'), [description, setDescription] = useState(''), [selected, setSelected] = useState<Service[]>([]), [owner, setOwner] = useState(me.user.id), [note, setNote] = useState(''), [reason, setReason] = useState(''), [fresh, setFresh] = useState(false);
  const [dedupKey] = useState(() => crypto.randomUUID()), key = useStatusKey(), cache = useQueryClient(), navigate = useNavigate();
  const services = useInfiniteQuery({ queryKey: ['incident-service-options'], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Service>>('/services?' + new URLSearchParams({ lifecycle: 'active', limit: '50', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const options = services.error ? [] : services.data?.pages.flatMap(page => page.items).filter(item => item.can_update_incident) ?? [];
  const mutation = useMutation({ mutationFn: () => {
    const body: Schemas['IncidentCreate'] = { title, severity, description, service_ids: selected.map(item => item.id), owner_id: owner, timeline_note: note, dedup_key: dedupKey, reason };
    return api<Incident>('/incidents', { method: 'POST', body, key: key(body) });
  }, onSuccess: async value => { await cache.invalidateQueries({ queryKey: ['incidents'] }); onClose(); navigate('/incidents/' + value.id); } });
  return <Dialog title="登记服务事件" onClose={onClose} busy={mutation.isPending}><form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}><ErrorNotice error={mutation.error} /><StatusConflict error={mutation.error} /><Field label="事件标题"><input required maxLength={200} value={title} onChange={event => setTitle(event.target.value)} /></Field><Field label="影响等级"><select value={severity} onChange={event => setSeverity(event.target.value as Incident['severity'])}>{Object.entries(severities).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></Field>
    <fieldset className="status-service-options"><legend>影响服务（1—20 项）</legend><ErrorNotice error={services.error} />{services.isPending ? <Busy label="正在读取可处置服务" /> : options.length ? options.map(item => <label className="checkbox-row" key={item.id}><input type="checkbox" checked={selected.some(value => value.id === item.id)} disabled={selected.length >= 20 && !selected.some(value => value.id === item.id)} onChange={event => { setSelected(current => event.target.checked ? [...current, item] : current.filter(value => value.id !== item.id)); setOwner(me.user.id); }} />{item.name}</label>) : !services.error && <p className="muted">尚无可登记事件的服务。</p>}{services.hasNextPage && !services.error && <button type="button" className="text-button" disabled={services.isFetchingNextPage} onClick={() => void services.fetchNextPage()}>加载更多可处置服务</button>}</fieldset>
    <IncidentOwner me={me} services={selected} owner={owner} setOwner={setOwner} /><Field label="事件说明"><textarea rows={4} maxLength={10000} value={description} onChange={event => setDescription(event.target.value)} /></Field><Field label="首次时间线记录"><textarea required rows={3} maxLength={10000} value={note} onChange={event => setNote(event.target.value)} /></Field><Field label="登记原因"><textarea required rows={2} maxLength={200} value={reason} onChange={event => setReason(event.target.value)} /></Field><p className="form-note">建立后固定标题、等级、负责人和影响范围。提交时会重新检查全部影响服务的当前权限。</p><MFAConfirmation onFresh={setFresh} /><div className="dialog-actions"><button type="button" className="button secondary" disabled={mutation.isPending} onClick={onClose}>取消</button><button className="button primary" disabled={!fresh || !selected.length || services.isError || mutation.isPending || isStatusConflict(mutation.error)}>确认登记事件</button></div></form></Dialog>;
}
function IncidentOwner({ me, services, owner, setOwner }: { me: Me; services: Service[]; owner: string; setOwner: (value: string) => void }) {
  const projects = [...new Set(services.flatMap(item => item.project_id ? [item.project_id] : []))].sort();
  const queries = useQueries({ queries: projects.map(id => ({ queryKey: ['members', id], queryFn: ({ signal }: { signal: AbortSignal }) => api<Schemas['ProjectMembers']>('/projects/' + id + '/members', { signal }) })) });
  let options = [{ id: me.user.id, display_name: me.user.display_name }];
  if (projects.length && queries.every(query => query.isSuccess)) options = queries[0].data!.items.filter(user => queries.every(query => query.data!.items.some(value => value.id === user.id)));
  const error = queries.find(query => query.error)?.error;
  return <><ErrorNotice error={error} /><Field label="事件负责人" hint="涉及多个项目时，只列出这些项目共有的成员；保存时再次核查有效身份。"><select required value={owner} onChange={event => setOwner(event.target.value)}><option value="">请选择负责人</option>{options.map(user => <option key={user.id} value={user.id}>{user.display_name}</option>)}</select></Field></>;
}
function IncidentUpdate({ incident, onClose }: { incident: Incident; onClose: () => void }) {
  const [base] = useState(incident), [state, setState] = useState(incident.state), [note, setNote] = useState(''), [resolution, setResolution] = useState(incident.resolution), [postmortem, setPostmortem] = useState(incident.postmortem), [reason, setReason] = useState(''), [fresh, setFresh] = useState(false);
  const key = useStatusKey(), cache = useQueryClient();
  const mutation = useMutation({ mutationFn: () => {
    const body: Schemas['IncidentPatch'] = { expected_version: base.version, state, timeline_note: note, resolution, postmortem, reason };
    return api<Incident>('/incidents/' + base.id, { method: 'PATCH', body, key: key(body) });
  }, onSuccess: async () => { await Promise.all([cache.invalidateQueries({ queryKey: ['incidents'] }), cache.invalidateQueries({ queryKey: ['incident', base.id] }), cache.invalidateQueries({ queryKey: ['incident-timeline', base.id] })]); onClose(); } });
  return <Dialog title={base.state === 'resolved' ? '追加事件复盘' : '更新事件进展'} onClose={onClose} busy={mutation.isPending}><form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}><ErrorNotice error={mutation.error} /><StatusConflict error={mutation.error} /><p><strong>{base.title}</strong> · 基于第 {base.version} 版</p><Field label="更新后的进度"><select value={state} disabled={base.state === 'resolved'} onChange={event => setState(event.target.value as Incident['state'])}>{Object.entries(states).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></Field><Field label="新增时间线记录"><textarea required rows={3} maxLength={10000} value={note} onChange={event => setNote(event.target.value)} /></Field><Field label="恢复说明"><textarea required={state === 'resolved'} rows={3} maxLength={10000} value={resolution} onChange={event => setResolution(event.target.value)} /></Field><Field label="复盘"><textarea rows={4} maxLength={10000} value={postmortem} onChange={event => setPostmortem(event.target.value)} /></Field><Field label="本次变更原因"><textarea required rows={2} maxLength={200} value={reason} onChange={event => setReason(event.target.value)} /></Field><p className="form-note">每次保存均追加记录并保留历史。已解决事件继续保留首次解决时间。</p><MFAConfirmation onFresh={setFresh} /><div className="dialog-actions"><button type="button" className="button secondary" disabled={mutation.isPending} onClick={onClose}>取消</button><button className="button primary" disabled={!fresh || mutation.isPending || isStatusConflict(mutation.error)}>确认追加进展</button></div></form></Dialog>;
}
