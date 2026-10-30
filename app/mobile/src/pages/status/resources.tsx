import { Text, View } from '@tarojs/components';
import { useEffect, useMemo, useState } from 'react';
import { resourceConfig, resourceCurrent, resourceDraft, resourceNames, resourcePercent, resourceReasons, resourceUsage, stampResource, type ResourceDraft, type ResourceSnapshot } from '@copypaste/contracts/host-resources';
import { useCollection, useRecord } from '../../platform/collection';
import { CommandNotice, MFAConfirmation, useCommand } from '../../platform/command';
import { MobileError, Scope, type Schemas } from '../../platform/session';
import { monotonicReader, statusTime } from '../../platform/status';
import { client } from '../../platform/taro';
import { Action, Busy, Empty, Fact, Notice, Section } from '../../ui';
import { SelectField, TextField } from '../business/controls';

type ResourceView = Schemas['ResourceView'];
export function HostResources({ service, me }: { service: string; me: Schemas['Me'] }) {
  const read = useMemo(() => monotonicReader(() => typeof performance !== 'undefined' && typeof performance.now === 'function' ? performance.now() : NaN), []);
  const [snapshot, setSnapshot] = useState<ResourceSnapshot>(), [error, setError] = useState<unknown>(), [now, setNow] = useState(read), [revision, setRevision] = useState(0), [loading, setLoading] = useState(true);
  const [editor, setEditor] = useState<ResourceView>(), [history, setHistory] = useState<'observations' | 'revisions'>();
  useEffect(() => {
    const scope = new Scope(); let flight = false, stopped = false; setSnapshot(undefined); setError(undefined); setLoading(true);
    async function poll() { if (scope.closed || flight || stopped) return; flight = true; const start = read(); try {
      const result = stampResource(await client.request<ResourceView>('/services/' + service + '/resource-monitor', { scope }), start, service);
      if (!scope.closed) { setSnapshot(result); setError(undefined); setNow(read()); }
    } catch (cause) { if (!scope.closed) { setSnapshot(undefined); setError(cause); if (cause instanceof MobileError && [401, 403, 404, 412].includes(cause.status)) { stopped = true; setEditor(undefined); setHistory(undefined); } } } finally { flight = false; if (!scope.closed) setLoading(false); } }
    void poll(); const timer = setInterval(() => void poll(), 5000), clock = setInterval(() => { if (!scope.closed) setNow(read()); }, 250);
    return () => { scope.close(); clearInterval(timer); clearInterval(clock); };
  }, [service, read, revision]);
  const value = snapshot?.value, current = resourceCurrent(snapshot, now), observation = current ? value?.observation : undefined;
  const reload = () => setRevision(n => n + 1);
  return <Section title='主机资源监控'><View className='cp-pad'><Notice error={error} />{loading ? <Busy /> : value && <>
    <Text className={'cp-state cp-state-' + (current && value.state === 'operational' ? 'success' : current && value.state === 'critical' ? 'danger' : current && value.state === 'degraded' ? 'warning' : 'neutral')}>{current ? resourceNames[value.state] : '资源待重新确认'}</Text>
    <Text className='cp-paragraph'>{current ? resourceReasons[value.state_reason] ?? '依据当前完整主机指标评估' : '上次指标已到重读时刻，请等待新的有效观测。'}</Text>
    {observation && <ResourceFacts value={observation} />}
    <Text className='cp-muted cp-small cp-paragraph'>CPU 为近五分钟平均；内存按可用量计算；磁盘同时核对空间与只读状态。主机资源与服务可用性分别评估，仅覆盖配置中的挂载点。</Text>
    <Fact label='受监控挂载点'><Text>{value.source?.mountpoints.join('、') ?? '尚未配置'}</Text></Fact><Fact label='配置版本'><Text>{value.source?.version ?? '尚未配置'}</Text></Fact><Fact label='最后评估'><Text>{statusTime(value.evaluated_at)}</Text></Fact>
    {value.can_manage && !editor && <Action secondary onClick={() => setEditor(value)}>配置资源监控</Action>}
  </>}
    <Action secondary onClick={reload}>刷新主机指标</Action><View className='cp-choice'><Action secondary onClick={() => setHistory('observations')}>资源观测历史</Action><Action secondary onClick={() => setHistory('revisions')}>资源配置历史</Action></View>
    {editor && <ResourceEditor key={service + ':' + me.authorization_version} base={editor} me={me} onClose={() => { setEditor(undefined); reload(); }} />}
    {history && <ResourceHistory key={service + ':' + history + ':' + me.authorization_version} service={service} kind={history} onClose={() => setHistory(undefined)} />}
  </View></Section>;
}
function ResourceFacts({ value }: { value: Schemas['ResourceObservation'] }) {
  return <><Fact label='CPU 使用率'><Text>{resourcePercent(value.cpu_usage_bp)}</Text></Fact><Fact label='内存使用率'><Text>{resourcePercent(resourceUsage(value.memory))}</Text></Fact><Fact label='内存可用／总量（字节）'><Text>{value.memory.available_bytes} / {value.memory.total_bytes}</Text></Fact><Fact label='采集时间'><Text>{statusTime(value.observed_at)}</Text></Fact>
    {value.filesystems.map(d => <View key={d.mountpoint} className='cp-editor-row'><Text className='cp-row-title'>磁盘 {d.mountpoint}</Text><Fact label='使用率'><Text>{resourcePercent(resourceUsage(d))}</Text></Fact><Fact label='可用／总量（字节）'><Text>{d.available_bytes} / {d.total_bytes}</Text></Fact><Text className={d.read_only ? 'cp-notice cp-danger' : 'cp-muted'}>{d.read_only ? '只读 · 严重告警' : '可读写'}</Text></View>)}
  </>;
}
const thresholds: readonly [keyof ResourceDraft, string][] = [['cpuWarning', 'CPU 警告阈值（%）'], ['cpuCritical', 'CPU 严重阈值（%）'], ['memoryWarning', '内存警告阈值（%）'], ['memoryCritical', '内存严重阈值（%）'], ['diskWarning', '磁盘警告阈值（%）'], ['diskCritical', '磁盘严重阈值（%）']];
function ResourceEditor({ base, me, onClose }: { base: ResourceView; me: Schemas['Me']; onClose(): void }) {
  const [draft, setDraft] = useState(() => resourceDraft(base.source)), [fresh, setFresh] = useState(false), [error, setError] = useState<unknown>();
  const command = useCommand<Schemas['Service']>(() => onClose(), 'host-resources:' + me.user.id + ':' + me.authorization_version + ':' + base.service_id + ':' + base.service_version, result => result.id === base.service_id && result.version === base.service_version + 1);
  const disabled = command.busy || command.blocked, change = (key: keyof ResourceDraft, text: string | boolean) => setDraft(old => ({ ...old, [key]: text }));
  function publish() { if (!fresh || disabled) return; try { const body = resourceConfig(draft, base); setError(undefined); void command.submit('/services/' + base.service_id + '/resource-monitor/revisions', body); } catch (cause) { setError(cause); } }
  return <Section title='发布资源监控配置'><View className='cp-pad'><Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} /><Text className='cp-notice cp-paragraph'>预填阈值仅为建议，明确发布后生效。新配置等待完整新指标，历史配置与证据继续保留。输入发生冲突时请返回刷新核对，不会自动重送。</Text>
    <TextField name='resource-name' label='监控名称' value={draft.name} onChange={v => change('name', v)} disabled={disabled} />
    <TextField name='resource-key' label='Ed25519 公钥' value={draft.publicKey} max={44} onChange={v => change('publicKey', v)} disabled={disabled} />
    <TextField name='resource-stale' label='采集断流阈值（秒）' value={draft.stale} max={3} onChange={v => change('stale', v)} disabled={disabled} />
    <TextField name='resource-mounts' label='受监控挂载点（每行一个，须列全）' value={draft.mounts} max={3216} multiline onChange={v => change('mounts', v)} disabled={disabled} />
    {thresholds.map(([key, label]) => <TextField key={key} name={'resource-' + key} label={label} value={String(draft[key])} max={6} onChange={v => change(key, v)} disabled={disabled} />)}
    <SelectField label='采集状态' value={draft.enabled ? 'on' : 'off'} options={[[ 'on', '启用' ], [ 'off', '停用' ]]} onChange={v => change('enabled', v === 'on')} disabled={disabled} />
    <TextField name='resource-reason' label='发布原因' value={draft.reason} onChange={v => change('reason', v)} disabled={disabled} multiline />
    <MFAConfirmation disabled={disabled} onFresh={setFresh} /><Action disabled={!fresh || disabled} loading={command.busy} onClick={publish}>确认发布资源配置</Action><Action secondary disabled={command.busy} onClick={onClose}>返回并重新读取配置</Action>
  </View></Section>;
}
function ResourceHistory({ service, kind, onClose }: { service: string; kind: 'observations' | 'revisions'; onClose(): void }) {
  const path = '/services/' + service + '/resource-monitor/', feed = useCollection<Schemas['ResourceObservation'] | Schemas['ResourceRevision']>(path + kind + '?limit=10'), [proof, setProof] = useState<string>();
  const evidence = useRecord<Schemas['ResourceProof']>(proof ? path + 'observations/' + proof + '/evidence' : null);
  return <Section title={kind === 'observations' ? '主机资源观测历史' : '资源配置历史'}><View className='cp-pad'><Text className='cp-notice cp-paragraph'>以下为历史事实，不代表当前主机状态。</Text><Notice error={feed.error} />{feed.loading ? <Busy /> : !feed.items.length && !feed.error ? <Empty>尚无历史记录。</Empty> : !feed.error && feed.items.map(row => 'cpu_usage_bp' in row ? <View key={row.id} className='cp-editor-row'><Text className='cp-row-title'>观测 {row.sequence} · {row.applied ? '已采纳' : '未覆盖当前指标：' + row.ignored_reason}</Text><ResourceFacts value={row} /><Fact label='接收于'><Text>{statusTime(row.received_at)}</Text></Fact><Text className='cp-code cp-paragraph'>{row.payload_sha256}</Text><Action secondary onClick={() => setProof(row.id)}>核对原始签名</Action></View> : <View key={row.revision_id} className='cp-editor-row'><Text className='cp-row-title'>配置第 {row.version} 版 · {row.is_current ? '当前修订' : '历史修订'}</Text><Text className='cp-paragraph'>{row.name} · {row.enabled ? '启用' : '停用'} · {row.stale_after_seconds} 秒</Text><Fact label='挂载点'><Text>{row.mountpoints.join('、')}</Text></Fact><Fact label='CPU 警告／严重'><Text>{resourcePercent(row.cpu.warning_bp)} / {resourcePercent(row.cpu.critical_bp)}</Text></Fact><Fact label='内存警告／严重'><Text>{resourcePercent(row.memory.warning_bp)} / {resourcePercent(row.memory.critical_bp)}</Text></Fact><Fact label='磁盘警告／严重'><Text>{resourcePercent(row.disk.warning_bp)} / {resourcePercent(row.disk.critical_bp)}</Text></Fact><Text className='cp-paragraph'>{row.reason} · {statusTime(row.created_at)}</Text></View>)}
    <Notice error={evidence.error} />{proof && evidence.loading ? <Busy /> : proof && evidence.value && <Text className='cp-notice cp-paragraph'>原始载荷摘要及 Ed25519 签名已校验：{evidence.value.id} · {evidence.value.payload_sha256}</Text>}{feed.next && !feed.error && <Action secondary loading={feed.moreBusy} onClick={() => void feed.more()}>加载更早资源记录</Action>}<Action secondary onClick={onClose}>返回主机资源</Action>
  </View></Section>;
}
