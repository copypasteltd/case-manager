import { useEffect, useRef, useState } from 'react';
import { NavLink } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { api, ApiError, type Me, type Schemas } from './api';
import { ErrorNotice, Field } from './components';
import { statusIsCurrent, statusNames, statusTime, statusTone, type StatusSnapshot } from './statusFreshness';

export function StatusNavigation() {
  return <nav className="status-navigation" aria-label="运行状态导航"><NavLink to="/services">服务目录</NavLink><NavLink to="/incidents">事件记录</NavLink></nav>;
}
export function useStatusClock() {
  const [now, setNow] = useState(() => performance.now());
  useEffect(() => { const timer = setInterval(() => setNow(performance.now()), 500); return () => clearInterval(timer); }, []);
  return now;
}
export function StatusBadge({ snapshot, now }: { snapshot: StatusSnapshot; now: number }) {
  const current = statusIsCurrent(snapshot, now), value = snapshot.value;
  return <span className={'badge ' + (current ? statusTone[value.state] : 'neutral')} title={'上次评估：' + statusTime(value.evaluated_at)}>{current ? statusNames[value.state] : '待重新确认'}</span>;
}
export function useStatusKey() {
  const current = useRef({ text: '', key: '' });
  return (body: unknown) => {
    const text = JSON.stringify(body);
    if (current.current.text !== text) current.current = { text, key: crypto.randomUUID() };
    return current.current.key;
  };
}
export function StatusConflict({ error }: { error: unknown }) {
  return isStatusConflict(error) ? <p className="notice neutral">资料或授权已变化。填写内容已保留，请关闭此窗口并重新核对当前版本；不会自动覆盖或重新提交。</p> : null;
}
export function isStatusConflict(error: unknown) {
  return error instanceof ApiError && (error.status === 409 || error.status === 412);
}
export function StatusOwner({ me, project, owner, setOwner, currentOwner }: { me: Me; project?: string | null; owner: string; setOwner: (value: string) => void; currentOwner?: string }) {
  const query = useQuery({ queryKey: ['members', project], enabled: !!project, queryFn: ({ signal }) => api<Schemas['ProjectMembers']>('/projects/' + project + '/members', { signal }) });
  const users = new Map<string, string>([[me.user.id, me.user.display_name]]);
  for (const user of query.data?.items ?? []) users.set(user.id, user.display_name);
  if (currentOwner && !users.has(currentOwner)) users.set(currentOwner, '当前负责人');
  return <><ErrorNotice error={query.error} /><Field label="负责人" hint={project ? '保存时将重新核对负责人是否仍为有效项目成员。' : '公司服务可先由本人负责；项目服务可选择当前项目成员。'}><select required value={owner} onChange={event => setOwner(event.target.value)}>{[...users].map(([id, name]) => <option key={id} value={id}>{name}</option>)}</select></Field></>;
}
export const sourceReasonNames: Record<string, string> = {
  SOURCE_DISABLED: '来源已停用', NO_OBSERVATION: '尚无有效观测', OBSERVATION_STALE: '观测已过期',
  RECEIPT_STALE: '接收已过期', CLOCK_SKEW: '时钟异常', INVALID_STATE: '状态待核对',
  LATE_SEQUENCE: '序号迟到，未覆盖当前状态', LATE_OBSERVATION: '观测时间迟到，未覆盖当前状态', STALE_SAMPLE: '样本已过期，未覆盖当前状态',
};
