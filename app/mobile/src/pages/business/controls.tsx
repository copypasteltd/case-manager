import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { Input, Picker, Textarea, semantic } from '../../platform/components';
import { useCollection } from '../../platform/collection';
import type { Schemas } from '../../platform/session';
import { Action, Busy, Empty, Field, Notice, Row, Section, State, bytes, date } from '../../ui';
import type { InputIssue } from '../../platform/business-input';

export function TextField({ name, label, value, onChange, disabled, multiline = false, max = 200, hint, issues = [], focus }: {
  name: string; label: string; value: string; onChange(value: string): void; disabled: boolean;
  multiline?: boolean; max?: number; hint?: string; issues?: InputIssue[]; focus?: string;
}) {
  const errors = issues.filter(issue => issue.path === name), id = 'oa-' + name;
  const accessible = process.env.TARO_ENV === 'h5' ? { 'aria-invalid': errors.length ? 'true' : 'false', 'aria-describedby': errors.length ? id + '-error' : undefined } : {};
  return <View id={id} className='cp-form-target'><Field label={label} hint={hint}>
    {multiline ? <Textarea ariaLabel={label} className='cp-textarea' value={value} maxlength={max} disabled={disabled} focus={focus === name} nativeProps={accessible} onInput={event => onChange(event.detail.value)} />
      : <Input ariaLabel={label} className='cp-input' value={value} maxlength={max} disabled={disabled} focus={focus === name} nativeProps={accessible} onInput={event => onChange(event.detail.value)} />}
    {errors.length > 0 && <View id={id + '-error'} {...semantic('alert')}><Text className='cp-field-error'>{errors.map(issue => issue.message).join(' ')}</Text></View>}
  </Field></View>;
}
export function SelectField({ label, value, options, onChange, disabled, hint }: {
  label: string; value: string; options: readonly (readonly [string, string])[]; onChange(value: string): void; disabled: boolean; hint?: string;
}) {
  const index = options.findIndex(item => item[0] === value);
  return <Field label={label} hint={hint}><Picker ariaLabel={label} mode='selector' range={options.map(item => item[1])} value={Math.max(0, index)} disabled={disabled} onChange={event => { const option = options[Number(event.detail.value)]; if (option) onChange(option[0]); }}>
    <View className='cp-picker'>{index >= 0 ? options[index][1] : '当前选择待核对'}</View>
  </Picker></Field>;
}
export function ChoiceList<T extends { id: string }>({ title, path, label, selected, onSelect, onClose, eligible, searchable = true }: {
  title: string; path: string; label(value: T): string; selected?: string; onSelect(value: T): void; onClose(): void; eligible?(value: T): boolean; searchable?: boolean;
}) {
  const [search, setSearch] = useState(''), [query, setQuery] = useState('');
  const items = useCollection<T>(path + (searchable && query ? '&q=' + encodeURIComponent(query) : ''));
  return <Section title={title}><View className='cp-pad'>{searchable && <><TextField name='choice-search' label='查找候选资料' value={search} onChange={setSearch} disabled={items.loading} />
    <Action secondary onClick={() => { setQuery(search.trim()); items.reload(); }}>查找候选资料</Action></>}<Notice error={items.error} />
    {items.loading ? <Busy /> : items.items.length ? items.items.map(item => <View key={item.id}>
      {eligible && !eligible(item) ? <Text className='cp-muted cp-small cp-paragraph'>{label(item)} · 当前不可选</Text> : <Row title={label(item)} subtitle={selected === item.id ? '当前所选' : undefined} onClick={() => onSelect(item)} />}
    </View>) : !items.error && <Empty>没有符合条件的可见候选，请调整查找或先登记资料。</Empty>}
    {items.next && <Action secondary loading={items.moreBusy} onClick={() => void items.more()}>加载更多候选</Action>}
    <Action secondary onClick={onClose}>返回表单，保留输入</Action>
  </View></Section>;
}
const rank = { internal: 0, project_restricted: 1, confidential: 2 };
export function referenceEligible(value: { classification: keyof typeof rank; project_id: string | null }, project: string, classification: keyof typeof rank) {
  return rank[value.classification] <= rank[classification] && (!value.project_id || value.project_id === project);
}
export function ReferenceScope({ project, company, onChange }: { project: string; company: boolean; onChange(value: boolean): void }) {
  return project ? <SelectField label='候选范围' value={company ? 'company' : 'project'} options={[[ 'project', '本项目资料' ], [ 'company', '公司公共资料' ]]} onChange={value => onChange(value === 'company')} disabled={false} /> : <Text className='cp-muted cp-small cp-paragraph'>从你有权查阅的公司资料中选择。</Text>;
}
export function referenceQuery(project: string, company: boolean) { return company || !project ? '&company_only=true' : '&project_id=' + project; }

export function FileChoice({ project, classification, onSelect, onClose }: { project: string; classification: keyof typeof rank; onSelect(file: Schemas['BusinessFileReference']): void; onClose(): void }) {
  const [company, setCompany] = useState(!project), [file, setFile] = useState<Schemas['File']>();
  return <><ReferenceScope project={project} company={company} onChange={value => { setCompany(value); setFile(undefined); }} />
    {file ? <VersionChoice file={file} onSelect={onSelect} onClose={() => setFile(undefined)} /> : <ChoiceList<Schemas['File']> title='选择依据文件' path={'/files?limit=20&state=active' + referenceQuery(project, company)} label={item => item.name} eligible={item => !!item.current_version_id && referenceEligible(item, project, classification)} onSelect={setFile} onClose={onClose} />}
    {file && <Action secondary onClick={onClose}>返回表单，保留输入</Action>}
  </>;
}
function VersionChoice({ file, onSelect, onClose }: { file: Schemas['File']; onSelect(file: Schemas['BusinessFileReference']): void; onClose(): void }) {
  const versions = useCollection<Schemas['FileVersion']>('/files/' + file.id + '/versions?limit=20');
  return <Section title='选择准确的正式版本'><View className='cp-pad'><Text className='cp-row-title'>{file.name}</Text><Notice error={versions.error} />
    {versions.loading ? <Busy /> : versions.items.length ? versions.items.map(version => {
      const safe = version.scan_state === 'clean' || version.scan_state === 'waived', eligible = safe && version.availability === 'available';
      return <View className='cp-fixed-file' key={version.id}><Text className='cp-row-title'>版本 {version.seq} · {version.original_name ?? '原文件名未提供'}</Text>
        <Text className='cp-muted cp-small'>{bytes(version.size_bytes)} · {date(version.created_at)}</Text><View className='cp-choice'><State value={version.scan_state} /><State value={version.availability} /></View>
        <Text className='cp-code'>{version.sha256}</Text>{version.scan_state === 'waived' && <Text className='cp-notice cp-paragraph'>此版本依据扫描例外放行，请核对资料风险。</Text>}
        <Action secondary disabled={!eligible} onClick={() => { if (eligible) onSelect({ file_id: file.id, version_id: version.id, seq: version.seq, name: version.original_name ?? '原文件名未提供', size_bytes: version.size_bytes, sha256: version.sha256, media_type: version.media_type, scan_state: version.scan_state as 'clean' | 'waived', scan_waiver_ref: version.scan_waiver_ref }); }}>使用固定版本 {version.seq}</Action>
      </View>;
    }) : !versions.error && <Empty>尚无可选正式版本。</Empty>}
    {versions.next && <Action secondary loading={versions.moreBusy} onClick={() => void versions.more()}>加载更早版本</Action>}
    {Boolean(versions.error) && <Action secondary onClick={versions.reload}>重新读取版本</Action>}
    <Action secondary onClick={onClose}>返回候选文件</Action>
  </View></Section>;
}
export function PartyChoice({ project, classification, onSelect, onClose }: { project: string; classification: keyof typeof rank; onSelect(party: Schemas['Party']): void; onClose(): void }) {
  const [company, setCompany] = useState(!project);
  return <><ReferenceScope project={project} company={company} onChange={setCompany} />
    <ChoiceList<Schemas['Party']> title='选择固定往来方修订' path={'/parties?state=active&limit=20' + referenceQuery(project, company)} label={item => item.name} eligible={item => referenceEligible(item, project, classification)} onSelect={onSelect} onClose={onClose} />
    <Text className='cp-muted cp-small cp-paragraph'>采用所选资料当前修订。后续往来方资料变动不会替换本次固定依据。</Text>
  </>;
}
