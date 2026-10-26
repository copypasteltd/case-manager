import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { Input, Picker } from './components';
import { useCollection } from './collection';
import { goVersion } from './page';
import type { Schemas } from './session';
import { Action, Busy, Empty, Field, Notice, Row, Section, State, bytes, date } from '../ui';

export type FixedFile = { file_id: string; version_id: string; name: string; seq: number; size_bytes: string; sha256: string };

export function FixedFiles({ files }: { files: FixedFile[] }) {
  return <>{files.map(file => <View className='cp-fixed-file' key={file.version_id}>
    <Text className='cp-row-title'>{file.name}</Text>
    <Text className='cp-muted cp-small'>固定版本 {file.seq} · {bytes(file.size_bytes)}</Text>
    <Text className='cp-code'>SHA-256 {file.sha256}</Text>
    <Action secondary onClick={() => goVersion(file.file_id, file.version_id)}>查看这个固定版本</Action>
  </View>)}</>;
}

export function FixedFilePicker({ projectID, value, onChange, disabled, optional = false, purpose = '交付' }: { projectID: string; value: FixedFile[]; onChange(files: FixedFile[]): void; disabled: boolean; optional?: boolean; purpose?: string }) {
  const [search, setSearch] = useState(''), [query, setQuery] = useState(''), [company, setCompany] = useState(false);
  const [selected, setSelected] = useState<Schemas['File']>();
  const files = useCollection<Schemas['File']>('/files?limit=20&state=active' + (company ? '&company_only=true' : '&project_id=' + projectID) + (query ? '&q=' + encodeURIComponent(query) : ''));
  return <View>
    <Text className='cp-muted cp-small cp-paragraph'>选择{optional ? 0 : 1}—20个正式版本。仅列出当前可用、扫描通过或已有扫描例外的候选；提交时服务器会再次核查。未上传的文件请先在文件中心完成上传。</Text>
    <Field label='查找文件'><Input ariaLabel={'查找' + purpose + '文件'} className='cp-input' value={search} maxlength={200} disabled={disabled} onInput={event => setSearch(event.detail.value)} />
      <Action secondary disabled={disabled} onClick={() => { setQuery(search.trim()); setSelected(undefined); files.reload(); }}>查找{purpose}文件</Action>
    </Field>
    <Field label='文件范围'><Picker ariaLabel='文件范围' mode='selector' range={['本项目文件', '公司公共文件']} value={company ? 1 : 0} disabled={disabled} onChange={event => { setCompany(Number(event.detail.value) === 1); setSelected(undefined); }}><View className='cp-picker'>{company ? '公司公共文件' : '本项目文件'}</View></Picker></Field>
    <Notice error={files.error} />{files.loading ? <Busy /> : <View className='cp-panel'>{files.items.length ? files.items.map(file => <Row key={file.id} title={file.name} subtitle='选择文件后再选择准确版本' onClick={() => { if (!disabled) setSelected(file); }} />) : !files.error && <Empty>没有符合条件的可见文件。</Empty>}</View>}
    {Boolean(files.error) && <Action secondary onClick={files.reload}>重新读取文件</Action>}
    {files.next && <Action secondary disabled={disabled} loading={files.moreBusy} onClick={() => void files.more()}>加载更多文件</Action>}
    {selected && <VersionChoices key={selected.id} file={selected} selected={value} disabled={disabled} onSelect={file => onChange([...value, file])} />}
    <Section title={'已选择的固定版本 · ' + value.length + '/20'}>{value.length ? value.map(file => <View className='cp-pad' key={file.version_id}>
      <Text className='cp-row-title'>{file.name}</Text><Text className='cp-muted cp-small'>固定版本 {file.seq} · {bytes(file.size_bytes)}</Text>
      <Text className='cp-code'>{file.sha256}</Text>
      <Action secondary disabled={disabled} onClick={() => onChange(value.filter(item => item.version_id !== file.version_id))}>移除此版本</Action>
    </View>) : <Empty>尚未选择正式版本。</Empty>}</Section>
  </View>;
}

function VersionChoices({ file, selected, onSelect, disabled }: { file: Schemas['File']; selected: FixedFile[]; onSelect(value: FixedFile): void; disabled: boolean }) {
  const versions = useCollection<Schemas['FileVersion']>('/files/' + file.id + '/versions?limit=20');
  return <Section title={file.name + ' · 正式版本'}><Notice error={versions.error} />{versions.loading ? <Busy /> : versions.items.map(version => {
    const eligible = version.availability === 'available' && ['clean', 'waived'].includes(version.scan_state);
    const picked = selected.some(item => item.version_id === version.id);
    return <View className='cp-pad' key={version.id}>
      <Text className='cp-row-title'>版本 {version.seq}{version.id === file.current_version_id ? ' · 当前版本' : ' · 历史版本'}</Text>
      <Text className='cp-muted cp-small'>{version.original_name ?? '未提供原始文件名'} · {bytes(version.size_bytes)} · {date(version.created_at)}</Text>
      <View className='cp-choice'><State value={version.availability} /><State value={version.scan_state} /></View>
      <Text className='cp-code'>{version.sha256}</Text>
      <Action secondary disabled={disabled || !eligible || picked || selected.length >= 20} onClick={() => onSelect({ file_id: file.id, version_id: version.id, name: version.original_name ?? file.name, seq: version.seq, size_bytes: version.size_bytes, sha256: version.sha256 })}>{picked ? '此版本已选择' : '选择固定版本 ' + version.seq}</Action>
    </View>;
  })}
    {!versions.loading && !versions.error && !versions.items.length && <Empty>这个文件尚无正式版本。</Empty>}
    {Boolean(versions.error) && <View className='cp-pad'><Action secondary onClick={versions.reload}>重新读取版本</Action></View>}
    {versions.next && <View className='cp-pad'><Action secondary loading={versions.moreBusy} disabled={disabled} onClick={() => void versions.more()}>加载更多版本</Action></View>}
  </Section>;
}
