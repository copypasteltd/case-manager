import { Text, View } from '@tarojs/components';
import Taro from '@tarojs/taro';
import { useEffect, useRef, useState } from 'react';
import { CommandNotice, MFAConfirmation, useCommand } from '../../platform/command';
import { useFormDraft } from '../../platform/form';
import { incidentCreate, incidentIssues, incidentPatch, incidentReceipt, type IncidentInput } from '../../platform/incident-input';
import { MobileError, Scope, type Schemas } from '../../platform/session';
import { client } from '../../platform/taro';
import { incidentNames, severityNames } from '../../platform/status';
import { Action, Empty, Fact, Notice, Row, Section } from '../../ui';
import { ChoiceList, SelectField, TextField } from '../business/controls';

export type IncidentSeed = { input: IncidentInput; reason: string };
export function IncidentEditor({ me, current, seed, onRebase, onReload, onCommitted, onCancel }: { me: Schemas['Me']; current?: Schemas['Incident']; seed?: IncidentSeed; onRebase?(value: Schemas['Incident'], seed: IncidentSeed): void; onReload?(): void; onCommitted(value: Schemas['Incident']): void; onCancel(): void }) {
  const [dedup] = useState(() => clientKey());
  const defaults: IncidentInput = seed?.input ?? { title: current?.title ?? '', description: current?.description ?? '', owner: current?.owner_id ?? me.user.id, severity: current?.severity ?? 'medium', state: current?.state ?? 'open', note: '', resolution: current?.resolution ?? '', postmortem: current?.postmortem ?? '', dedup: current?.dedup_key ?? dedup };
  const draft = useFormDraft(me, { kind: current ? 'incident-update' : 'incident-create', id: current?.id ?? 'new-incident', version: current?.version ?? 1 }, current ? 'update' : 'create', { incident: defaults });
  const input = draft.fields.incident ?? defaults;
  useEffect(() => { if (seed) draft.update({ incident: seed.input, reason: seed.reason }); }, []);
  const [services, setServices] = useState<Schemas['Service'][]>([]), [owner, setOwner] = useState({ id: me.user.id, name: me.user.display_name });
  const [picking, setPicking] = useState<'services' | 'owner'>(), [fresh, setFresh] = useState(false), [issues, setIssues] = useState<ReturnType<typeof incidentIssues>>([]), [focus, setFocus] = useState<string>();
  const intent = useRef<Schemas['IncidentCreate'] | Schemas['IncidentPatch']>();
  const conflictScope = useRef<Scope>(), [latest, setLatest] = useState<Schemas['Incident']>(), [readError, setReadError] = useState<unknown>(), [reading, setReading] = useState(false);
  useEffect(() => () => conflictScope.current?.close(), []);
  const command = useCommand<Schemas['Incident']>(value => { draft.forget(); onCommitted(value); }, draft.binding, value => !!intent.current && incidentReceipt(value, intent.current, current));
  const disabled = command.busy || command.blocked;
  async function readLatest() {
    if (!current || reading) return;
    conflictScope.current?.close(); const scope = new Scope(); conflictScope.current = scope;
    setReading(true); setLatest(undefined); setReadError(undefined);
    try {
      const who = await client.request<Schemas['Me']>('/me', { scope });
      if (who.user.id !== me.user.id || who.authorization_version !== me.authorization_version) { onReload?.(); return; }
      const value = await client.request<Schemas['Incident']>('/incidents/' + current.id, { scope });
      if (value.id !== current.id || !Number.isSafeInteger(value.version) || value.version <= current.version || !value.can_update) throw new MobileError('CURRENT_RECORD_UNCONFIRMED', '当前修订或处理权限尚未核对，请重新读取当前权限与资料。');
      if (!scope.closed) setLatest(value);
    } catch (cause) {
      if (!scope.closed) { if (cause instanceof MobileError && [401, 403, 404].includes(cause.status)) onReload?.(); else setReadError(cause); }
    } finally { if (!scope.closed) setReading(false); }
  }
  function change(patch: Partial<IncidentInput>) { draft.update({ incident: { ...input, ...patch } }); setFocus(undefined); }
  function locate(path: string) { setFocus(path); void Taro.pageScrollTo({ selector: '#oa-' + path, duration: 150 }).catch(() => {}); }
  function save() {
    const ids = services.map(service => service.id), errors = incidentIssues(input, draft.fields.reason, ids, current);
    if (!current && owner.id !== input.owner) errors.push({ path: 'owner', label: '事件负责人', message: '请重新选择已保存输入中的负责人。' });
    setIssues(errors); if (errors.length) { locate(errors[0].path); return; }
    if (!fresh || disabled) return;
    const body = current ? incidentPatch(input, draft.fields.reason, current.version) : incidentCreate(input, draft.fields.reason, ids);
    intent.current = body; void command.submit('/incidents' + (current ? '/' + current.id : ''), body, current ? 'PATCH' : 'POST');
  }
  if (picking === 'services') return <ChoiceList<Schemas['Service']> title='选择受影响服务' path='/services?limit=20&lifecycle=active' label={service => service.name} eligible={service => service.can_update_incident && services.length < 20 && !services.some(selected => selected.id === service.id)} onSelect={service => { setServices(old => [...old, service]); setPicking(undefined); }} onClose={() => setPicking(undefined)} />;
  if (picking === 'owner') return <ChoiceList<Schemas['User']> title='选择事件负责人' path='/users?limit=20' searchable={false} label={user => user.display_name} selected={input.owner} eligible={user => user.status === 'active'} onSelect={user => { change({ owner: user.id }); setOwner({ id: user.id, name: user.display_name }); setPicking(undefined); }} onClose={() => setPicking(undefined)} />;
  return <Section title={current ? '追加事件进展' : '登记服务事件'}><View className='cp-pad'>
    {draft.restored && <Text className='cp-notice cp-paragraph'>已恢复此会话中未提交的文字输入。影响服务和两步验证需要重新操作；所用权限与事件修订已重新读取。</Text>}
    {!draft.retained && <Text className='cp-notice cp-paragraph'>本页输入超出会话草稿保留范围，离开前请妥善整理。</Text>}
    <Notice error={command.error} /><CommandNotice blocked={command.blocked} />
    {current && command.error instanceof MobileError && [409, 412].includes(command.error.status) && <View className='cp-status-proof'>
      <Text className='cp-paragraph'>原输入与修订 {current.version} 仍保留。先读取最新记录并逐项比较，再明确选择是否继续；不会自动重送。</Text>
      <Action secondary loading={reading} onClick={() => void readLatest()}>读取最新事件用于比较</Action><Notice error={readError} />
      {latest && <><Fact label='服务器最新修订'><Text>{latest.version} · {incidentNames[latest.state]}</Text></Fact><Fact label='服务器最新解决说明'><Text className='cp-status-text'>{latest.resolution || '尚未登记'}</Text></Fact><Fact label='服务器最新复盘'><Text className='cp-status-text'>{latest.postmortem || '尚未登记'}</Text></Fact>
        <Text className='cp-notice cp-paragraph'>继续将保留你在下方填写的文字，以最新修订作为本次更新依据。请核对差异，并重新完成两步验证；已解决事件不会重新打开。</Text>
        <Action secondary onClick={() => onRebase?.(latest, { input: { ...input, state: latest.state === 'resolved' ? 'resolved' : input.state }, reason: draft.fields.reason })}>保留我的文字，以最新修订继续核对</Action>
      </>}
    </View>}
    {issues.length > 0 && <View className='cp-notice cp-danger'><Text>请先核对以下内容：</Text>{issues.map((issue, index) => <Action key={issue.path + index} secondary onClick={() => locate(issue.path)}>{issue.message}</Action>)}</View>}
    {!current ? <>
      <TextField name='title' label='事件标题' value={input.title} onChange={title => change({ title })} disabled={disabled} issues={issues} focus={focus} />
      <TextField name='description' label='事件影响描述' value={input.description} onChange={description => change({ description })} disabled={disabled} multiline max={10000} issues={issues} focus={focus} />
      <SelectField label='严重程度' value={input.severity} options={Object.entries(severityNames)} disabled={disabled} onChange={severity => change({ severity: severity as IncidentInput['severity'] })} />
      <View id='oa-owner'><Fact label='事件负责人'><Text>{owner.id === input.owner ? owner.name : '已保存的负责人需要重新选择'}</Text></Fact>{me.actions.includes('user.read') && <Action secondary disabled={disabled} onClick={() => setPicking('owner')}>选择事件负责人</Action>}</View>
      <View id='oa-services'><Text className='cp-row-title'>受影响服务 · {services.length}／20</Text>
        {services.map(service => <Row key={service.id} title={service.name} subtitle='点击移除此项' onClick={() => { if (!disabled) setServices(items => items.filter(item => item.id !== service.id)); }} />)}
        {!services.length && <Empty>请明确选择全部受影响服务。只显示当前有权处理的候选。</Empty>}
        <Action secondary disabled={disabled || services.length >= 20} onClick={() => setPicking('services')}>添加受影响服务</Action>
      </View>
    </> : <><Text className='cp-muted cp-small cp-paragraph'>基于当前修订 {current.version} 追加记录。影响范围、负责人和事件标题保持登记时的事实；时间线保留每次变化。</Text>
      <SelectField label='事件处理阶段' value={input.state} options={Object.entries(incidentNames).filter(([key]) => current.state !== 'resolved' || key === 'resolved')} disabled={disabled} onChange={state => change({ state: state as IncidentInput['state'] })} />
      <TextField name='resolution' label='恢复与解决说明' value={input.resolution} onChange={resolution => change({ resolution })} disabled={disabled} multiline max={10000} hint='标记已解决时必填，记录实际恢复依据。' issues={issues} focus={focus} />
      <TextField name='postmortem' label='事件复盘' value={input.postmortem} onChange={postmortem => change({ postmortem })} disabled={disabled} multiline max={10000} issues={issues} focus={focus} />
    </>}
    <TextField name='note' label={current ? '本次时间线说明' : '首次时间线记录'} value={input.note} onChange={note => change({ note })} disabled={disabled} multiline max={10000} issues={issues} focus={focus} />
    <TextField name='reason' label='操作依据' value={draft.fields.reason} onChange={reason => draft.update({ reason })} disabled={disabled} max={200} issues={issues} focus={focus} />
    <MFAConfirmation disabled={disabled} onFresh={setFresh} />
    <Action loading={command.busy} disabled={command.blocked || !fresh} onClick={save}>{current ? '确认追加事件进展' : '确认登记事件'}</Action>
    <Action secondary disabled={command.busy} onClick={onCancel}>返回，保留未提交输入</Action>
  </View></Section>;
}
function clientKey() { return client.operationKey(); }
