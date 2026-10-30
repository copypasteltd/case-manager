import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { client } from '../../platform/taro';
import { MobileError, type Schemas } from '../../platform/session';
import { officeWritable, queueOfficeArtifact } from '../../platform/office-upload';
import { Action, Check, Notice, Section } from '../../ui';
import { SelectField, TextField } from '../business/controls';
import { checkedOfficeArtifact, officeChanges, officeDocument, officeHint, officeKinds, type OfficeKind } from '@copypaste/contracts/office-author';

export function OfficePanel({ folder, file }: { folder?: Schemas['Folder']; file?: Schemas['File'] }) {
  const [open, setOpen] = useState(false), [busy, setBusy] = useState(false), [error, setError] = useState<unknown>();
  const [kind, setKind] = useState<OfficeKind>('docx'), [name, setName] = useState(''), [title, setTitle] = useState(''), [content, setContent] = useState(''), [table, setTable] = useState('');
  const [basis, setBasis] = useState<Schemas['OfficeRead']>(), [values, setValues] = useState<Record<string, Schemas['OfficeCell']>>({}), [page, setPage] = useState(0), [confirmed, setConfirmed] = useState(false), [artifact, setArtifact] = useState<Schemas['OfficeArtifact']>();
  const writable = officeWritable && (file ? file.can_update : folder?.can_upload);
  if (file ? file.state !== 'active' || !file.current_version_id || !/\.(docx|xlsx|pptx)$/i.test(file.name) : !writable) return null;
  async function action(fn: () => Promise<void>) { setBusy(true); setError(undefined); try { await fn(); } catch (e) { setError(e instanceof MobileError ? e : new MobileError('OFFICE_UNCONFIRMED', e instanceof Error ? e.message : '文档操作未确认，请核对。')); } finally { setBusy(false); } }
  async function read() { const value = await client.request<Schemas['OfficeRead']>('/files/' + file!.id + '/versions/' + file!.current_version_id + '/office'); setBasis(value); setValues({}); setPage(0); setArtifact(undefined); }
  async function render() {
    const body = file ? { expected_version: basis!.file.version, base_version_id: basis!.version_id, source_sha256: basis!.source_sha256, workspace_sha256: basis!.workspace.sha256, changes: officeChanges(basis!.workspace, values) } : { expected_version: folder!.version, name, document: officeDocument(kind, title, content, table) };
    setArtifact(checkedOfficeArtifact(await client.request<Schemas['OfficeArtifact']>((file ? '/files/' + file.id : '/folders/' + folder!.id) + '/office-render', { method: 'POST', body, key: client.operationKey() }))); setConfirmed(false);
  }
  if (!open) return <Action secondary onClick={() => { setOpen(true); if (file) void action(read); }}>{file ? writable ? '编辑 Office 内容' : '查阅 Office 内容片段' : '新建 Office 文件'}</Action>;
  const units = basis?.workspace.units ?? [];
  return <Section title={file ? writable ? '编辑 Office 内容' : '查阅 Office 内容片段' : '新建 Office 文件'}><View className='cp-pad'><Notice error={error} />
    <Text className='cp-muted cp-small'>生成候选文件后，核对并加入上传队列；扫描和完整校验通过后人工提交正式版本。原件保持独立版本。</Text>
    {artifact ? <><Text className='cp-row-title'>{artifact.name}</Text>{artifact.workspace.warnings.map(text => <Text key={text}>{text}</Text>)}{artifact.workspace.units.slice(0, 10).map(unit => <Text key={unit.id}>{unit.group} · {unit.label}：{unit.value}</Text>)}<Text className='cp-code' selectable>{artifact.sha256}</Text><Check checked={confirmed} disabled={busy} onChange={setConfirmed}>我已核对候选内容，确认加入上传队列。</Check><Action disabled={!confirmed} loading={busy} onClick={() => void action(async () => { await queueOfficeArtifact(artifact); setArtifact(undefined); setOpen(false); })}>加入上传队列</Action><Action secondary disabled={busy} onClick={() => setArtifact(undefined)}>返回修改</Action></> : <>
      {file ? <><Action secondary disabled={busy} onClick={() => void action(read)}>重新读取依据并清空修改</Action>{basis?.workspace.warnings.map(text => <Text key={text}>{text}</Text>)}
        {units.slice(page * 20, page * 20 + 20).map(unit => writable && !unit.read_only ? <TextField key={unit.id} name={'office-' + unit.id} label={unit.group + ' · ' + unit.label + '（' + unit.kind + '）'} value={values[unit.id]?.value ?? unit.value} onChange={value => setValues(old => ({ ...old, [unit.id]: { kind: unit.kind, value } }))} disabled={busy} multiline max={8192} /> : <View key={unit.id}><Text className='cp-label'>{unit.group} · {unit.label}{unit.read_only ? ' · 只读' : ''}</Text><Text selectable>{unit.value}</Text></View>)}
        {units.length > 20 && <><Text>{page + 1} / {Math.ceil(units.length / 20)}</Text><Action secondary disabled={busy || page === 0} onClick={() => setPage(p => p - 1)}>上一页</Action><Action secondary disabled={busy || (page + 1) * 20 >= units.length} onClick={() => setPage(p => p + 1)}>下一页</Action></>}
      </> : <>
        <SelectField label='文件类型' value={kind} options={Object.entries(officeKinds) as [string,string][]} onChange={value => setKind(value as OfficeKind)} disabled={busy} />
        <TextField name='office-name' label='文件名称' value={name} onChange={setName} disabled={busy} max={180} />
        <TextField name='office-title' label='文档标题' value={title} onChange={setTitle} disabled={busy} max={256} />
        <TextField name='office-content' label={'文档内容 · ' + officeHint[kind]} value={content} onChange={setContent} disabled={busy} multiline max={100000} />
        {kind === 'docx' && <TextField name='office-table' label='可选表格：Tab 分列，换行分行' value={table} onChange={setTable} disabled={busy} multiline max={50000} />}
      </>}
      {writable && <Action loading={busy} disabled={file ? !basis || !officeChanges(basis.workspace, values).length : !name.trim()} onClick={() => void action(render)}>生成并核对候选文件</Action>}
    </>}
    <Action secondary disabled={busy} onClick={() => { setOpen(false); setBasis(undefined); setValues({}); setArtifact(undefined); }}>关闭文档工作区</Action>
  </View></Section>;
}
