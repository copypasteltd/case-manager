import type { components } from './api';
type S = components['schemas'];
export type ResourceDraft = { name: string; publicKey: string; enabled: boolean; stale: string; mounts: string; cpuWarning: string; cpuCritical: string; memoryWarning: string; memoryCritical: string; diskWarning: string; diskCritical: string; reason: string };
export const resourceNames = { operational: '资源正常', degraded: '资源告警', critical: '资源严重告警', unknown: '资源待确认' } as const;
export const resourceReasons: Record<string, string> = { NOT_CONFIGURED: '尚未配置主机指标', SOURCE_DISABLED: '资源来源已停用', SERVICE_ARCHIVED: '服务已归档', NO_OBSERVATION: '当前配置尚无有效指标', CLOCK_SKEW: '采集时钟需核对', OBSERVATION_STALE: '指标已过期或采集断流' };
export type ResourceSnapshot = { value: S['ResourceView']; startedAt: number; expiresAt: number };
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const byte = (s: string) => typeof s === 'string' && /^(0|[1-9][0-9]{0,18})$/.test(s) && (s.length < 19 || s <= '9223372036854775807');
const capacity = (m: S['ResourceMemory']) => !!m && byte(m.total_bytes) && m.total_bytes !== '0' && byte(m.available_bytes) && (m.available_bytes.length < m.total_bytes.length || m.available_bytes.length === m.total_bytes.length && m.available_bytes <= m.total_bytes);
export function resourceUsage(m: S['ResourceMemory']) {
  if (!capacity(m)) throw new Error('资源容量证据无效。');
  const total = BigInt(m.total_bytes), available = BigInt(m.available_bytes);
  return Number(((total - available) * BigInt(10000) + total - BigInt(1)) / total);
}
export const resourcePercent = (bp: number) => (bp / 100).toFixed(2) + '%';
export function stampResource(value: S['ResourceView'], startedAt: number, service: string): ResourceSnapshot {
  if (!value || value.service_id !== service || !uuid.test(service) || !Number.isSafeInteger(value.service_version) || value.service_version < 1 || !Object.prototype.hasOwnProperty.call(resourceNames, value.state) || typeof value.can_manage !== 'boolean') throw new Error('主机指标响应与当前服务不一致。');
  const source = value.source, o = value.observation;
  if (source && (!uuid.test(source.id) || !uuid.test(source.revision_id) || !Array.isArray(source.mountpoints) || source.mountpoints.length < 1 || source.mountpoints.length > 16 || new Set(source.mountpoints).size !== source.mountpoints.length)) throw new Error('资源来源依据不完整。');
  if (value.state === 'unknown' ? o !== null : !source || !o || !source.enabled || o.source_id !== source.id || o.source_revision_id !== source.revision_id || !o.applied || !!o.ignored_reason || !uuid.test(o.id) || !/^[a-f0-9]{64}$/.test(o.payload_sha256) || !Number.isInteger(o.cpu_usage_bp) || o.cpu_usage_bp < 0 || o.cpu_usage_bp > 10000 || !capacity(o.memory) || !Array.isArray(o.filesystems) || o.filesystems.length !== source.mountpoints.length || new Set(o.filesystems.map(d => d.mountpoint)).size !== source.mountpoints.length || o.filesystems.some(d => !source.mountpoints.includes(d.mountpoint) || !capacity(d) || typeof d.read_only !== 'boolean')) throw new Error('资源指标证据不完整，请重新读取。');
  const lifetime = Date.parse(value.recheck_at) - Date.parse(value.evaluated_at);
  const safe = Number.isFinite(startedAt) && startedAt >= 0 && Number.isFinite(lifetime) && lifetime > 0;
  return { value, startedAt, expiresAt: safe ? startedAt + Math.min(lifetime, 10000) : 0 };
}
export function resourceCurrent(snapshot: ResourceSnapshot | undefined, now: number) { return !!snapshot && Number.isFinite(now) && now >= snapshot.startedAt && now < snapshot.expiresAt; }
export function resourceDraft(source?: S['ResourceSource'] | null): ResourceDraft {
  return { name: source?.name ?? '主机资源', publicKey: source?.public_key ?? '', enabled: source?.enabled ?? true, stale: String(source?.stale_after_seconds ?? 90), mounts: source?.mountpoints.join('\n') ?? '/',
    cpuWarning: String((source?.cpu.warning_bp ?? 8000) / 100), cpuCritical: String((source?.cpu.critical_bp ?? 9500) / 100), memoryWarning: String((source?.memory.warning_bp ?? 8000) / 100), memoryCritical: String((source?.memory.critical_bp ?? 9500) / 100), diskWarning: String((source?.disk.warning_bp ?? 8000) / 100), diskCritical: String((source?.disk.critical_bp ?? 9500) / 100), reason: '' };
}
export function resourceConfig(d: ResourceDraft, base: S['ResourceView']): S['ResourceConfigInput'] {
  const percent = (text: string) => { if (!/^(0|[1-9][0-9]{0,2})(\.[0-9]{1,2})?$/.test(text)) throw new Error('阈值须为最多两位小数的百分比。'); const [n, f = ''] = text.split('.'); return Number(n) * 100 + Number(f.padEnd(2, '0')); };
  const threshold = (warning: string, critical: string) => { const w = percent(warning), c = percent(critical); if (w < 1 || w >= c || c > 10000) throw new Error('警告阈值须低于严重阈值，范围为0.01%至100%。'); return { warning_bp: w, critical_bp: c }; };
  const mounts = d.mounts.replace(/\r\n?/g, '\n').split('\n').filter(x => x !== '');
  if (!d.name.trim() || [...d.name].length > 200 || /[\r\n\x00]/.test(d.name) || !/^[A-Za-z0-9+/]{43}=$/.test(d.publicKey) || !/^[0-9]{2,3}$/.test(d.stale) || Number(d.stale) < 45 || Number(d.stale) > 180 || !d.reason.trim() || [...d.reason].length > 200) throw new Error('请核对名称、公钥、45至180秒的过期阈值和发布原因。');
  if (mounts.length < 1 || mounts.length > 16 || new Set(mounts).size !== mounts.length || mounts.some(p => !p.startsWith('/') || [...p].length > 200 || /[\r\n\t\x00\\]/.test(p) || p !== '/' && (p.endsWith('/') || p.includes('//') || p.split('/').some(c => c === '.' || c === '..')))) throw new Error('请每行填写一个唯一、规范的Linux绝对挂载路径，最多16个。');
  return { name: d.name.trim(), public_key: d.publicKey, enabled: d.enabled, stale_after_seconds: Number(d.stale), mountpoints: mounts, cpu: threshold(d.cpuWarning, d.cpuCritical), memory: threshold(d.memoryWarning, d.memoryCritical), disk: threshold(d.diskWarning, d.diskCritical), reason: d.reason.trim(), expected_version: base.service_version, expected_source_version: base.source?.version ?? 0, base_revision_id: base.source?.revision_id ?? null };
}
