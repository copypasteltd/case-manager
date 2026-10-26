import { Text, View } from '@tarojs/components';
import { useEffect, useState } from 'react';
import { useCollection, useRecord } from '../../platform/collection';
import { MobileError, Scope, type Schemas } from '../../platform/session';
import { client } from '../../platform/taro';
import { readVersion } from '../../platform/page';
import type { Source } from '../../platform/post-input';
import { Action, Busy, Empty, Notice, Row, Section, State, bytes } from '../../ui';
import { SelectField, TextField } from '../business/controls';

const eligible = (v: Schemas['FileVersion']) => v.availability === 'available' && ['clean', 'waived'].includes(v.scan_state);

export function SourceCard({ source, versionID, onRemove, disabled = false }: { source?: Source; versionID?: string; onRemove?(): void; disabled?: boolean }) {
  const [value, setValue] = useState<{ file: Schemas['File']; version: Schemas['FileVersion'] }>(), [error, setError] = useState<unknown>();
  useEffect(() => {
    const scope = new Scope(); setValue(undefined); setError(undefined);
    void (async () => {
      let version: Schemas['FileVersion'], file: Schemas['File'];
      if (source) {
        file = await client.request<Schemas['File']>('/files/' + source.id, { scope });
        const id = source.version_id ?? file.current_version_id;
        if (!id) throw new MobileError('NO_FILE_VERSION', '附件暂时没有可查阅版本。');
        version = await client.request<Schemas['FileVersion']>('/files/' + file.id + '/versions/' + id, { scope });
        if (version.id !== id || file.id !== source.id) throw new MobileError('INVALID_RESPONSE', '附件版本尚未核对。');
      } else {
        version = await client.request<Schemas['FileVersion']>('/file-versions/' + versionID, { scope });
        if (version.id !== versionID) throw new MobileError('INVALID_RESPONSE', '固定附件版本尚未核对。');
        file = await client.request<Schemas['File']>('/files/' + version.file_id, { scope });
      }
      if (version.file_id !== file.id || !eligible(version)) throw new MobileError('FILE_UNAVAILABLE', '此附件当前无法查阅。');
      if (!scope.closed) setValue({ file, version });
    })().catch(cause => { if (!scope.closed) setError(cause); });
    return () => { scope.close(); };
  }, [source?.id, source?.version_id, versionID]);
  return <View className='cp-pad'><Text className='cp-muted cp-small'>{source && !source.version_id ? '跟随当前版本 · 每次查阅重新解析' : '固定版本 · 原始依据'}</Text>
    {error ? <Text className='cp-notice'>附件当前不可用或无权查阅；文件名称不予展示。</Text> : value ? <>
      <Text className='cp-row-title'>{value.version.original_name ?? value.file.name}</Text><Text className='cp-muted cp-small'>版本 {value.version.seq} · {bytes(value.version.size_bytes)}</Text>
      <Text className='cp-code' selectable>SHA-256 {value.version.sha256}</Text>
      <Action secondary onClick={() => readVersion(value.file.id, value.version.id)}>查阅此附件内容</Action>
    </> : <Busy />}
    {onRemove && <Action secondary disabled={disabled} onClick={onRemove}>移除此附件</Action>}
  </View>;
}

export function Sources({ sources }: { sources: Source[] }) {
  const [limit, setLimit] = useState(20);
  return <>{sources.slice(0, limit).map((s, i) => <SourceCard key={i + ':' + s.id + ':' + s.version_id} source={s} />)}
    {sources.length > limit && <Action secondary onClick={() => setLimit(limit + 20)}>显示更多附件</Action>}</>;
}

export function SourcePicker({ projectID, value, onChange, disabled, fixedOnly = false, max = 100 }: {
  projectID: string; value: Source[]; onChange(value: Source[]): void; disabled: boolean; fixedOnly?: boolean; max?: number;
}) {
  const [range, setRange] = useState(projectID ? 'project' : 'company'), [search, setSearch] = useState(''), [query, setQuery] = useState('');
  const [selected, setSelected] = useState<Schemas['File']>(), [limit, setLimit] = useState(20);
  const files = useCollection<Schemas['File']>('/files?limit=20&state=active' + (range === 'project' ? '&project_id=' + projectID : range === 'company' ? '&company_only=true' : '') + (query ? '&q=' + encodeURIComponent(query) : ''));
  function add(source: Source) {
    if (!disabled && value.length < max && !value.some(s => s.id === source.id && (s.version_id ?? null) === (source.version_id ?? null))) onChange([...value, source]);
  }
  return <Section title={'知识附件 · ' + value.length + '/' + max}><View className='cp-pad'>
    <Text className='cp-muted cp-small'>附件权限独立核查。{fixedOnly ? '评论仅接受固定版本。' : '跟随版本会随文件更新；固定版本保留当时依据。'}提交时服务器再次检查可用性。</Text>
    <SelectField label='附件文件范围' value={range} disabled={disabled} options={[...(projectID ? [['project', '本项目文件'] as const] : []), ['company', '公司公共文件'], ['all', '全部当前可见文件']]} onChange={v => { setRange(v); setSelected(undefined); }} />
    <TextField name='post-file-search' label='查找知识附件' value={search} disabled={disabled} onChange={setSearch} />
    <Action secondary disabled={disabled} onClick={() => { setQuery(search.trim()); setSelected(undefined); files.reload(); }}>查找附件文件</Action>
    <Notice error={files.error} />{files.loading ? <Busy /> : files.items.length ? files.items.map(file => <Row key={file.id} title={file.name} subtitle='选择引用方式和正式版本' onClick={() => { if (!disabled) setSelected(file); }} />) : !files.error && <Empty>没有可见候选文件。</Empty>}
    {files.next && <Action secondary disabled={disabled} loading={files.moreBusy} onClick={() => void files.more()}>更多附件文件</Action>}
    {selected && <VersionChoices key={selected.id} file={selected} fixedOnly={fixedOnly} onSelect={add} disabled={disabled || value.length >= max} picked={value} />}
    <Text className='cp-section-title'>已选附件 · 顺序与引用方式</Text>
    {value.slice(0, limit).map((source, i) => <SourceCard key={i + ':' + source.id + ':' + source.version_id} source={source} disabled={disabled} onRemove={() => onChange(value.filter((_, at) => at !== i))} />)}
    {value.length > limit && <Action secondary onClick={() => setLimit(limit + 20)}>显示更多已选附件</Action>}
    {!value.length && <Empty>尚未引用附件。</Empty>}
  </View></Section>;
}

function VersionChoices({ file, fixedOnly, onSelect, disabled, picked }: { file: Schemas['File']; fixedOnly: boolean; onSelect(source: Source): void; disabled: boolean; picked: Source[] }) {
  const history = useCollection<Schemas['FileVersion']>('/files/' + file.id + '/versions?limit=20');
  const current = useRecord<Schemas['FileVersion']>(file.current_version_id ? '/files/' + file.id + '/versions/' + file.current_version_id : null);
  return <Section title={file.name + ' · 引用方式'}><View className='cp-pad'>
    {!fixedOnly && <><Notice error={current.error} />{current.loading ? <Busy /> : <Action secondary disabled={disabled || !current.value || !eligible(current.value) || picked.some(s => s.id === file.id && !s.version_id)} onClick={() => onSelect({ type: 'file', id: file.id })}>添加跟随当前版本</Action>}</>}
    <Notice error={history.error} />{history.loading ? <Busy /> : history.items.map(v => <View key={v.id} className='cp-pad'>
      <Text className='cp-row-title'>固定版本 {v.seq} · {v.original_name ?? '原名未提供'}</Text><Text className='cp-muted cp-small'>{bytes(v.size_bytes)}</Text>
      <State value={v.availability} /><State value={v.scan_state} /><Text className='cp-code' selectable>{v.sha256}</Text>
      <Action secondary disabled={disabled || !eligible(v) || picked.some(s => s.version_id === v.id)} onClick={() => onSelect({ type: 'file', id: file.id, version_id: v.id })}>引用固定版本 {v.seq}</Action>
    </View>)}
    {history.next && <Action secondary loading={history.moreBusy} disabled={disabled} onClick={() => void history.more()}>更多历史版本</Action>}
  </View></Section>;
}
