import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useMutation } from '@tanstack/react-query';
import { api, ensureDownloadSession, type Schemas } from './api';
import { Dialog, Field, ErrorNotice } from './components';
import { useUploadQueue } from './UploadQueue';
import { sourceFingerprint } from './upload';
import { checkedOfficeArtifact, officeChanges, officeDocument, officeHint, officeKinds, type OfficeKind } from '@copypaste/contracts/office-author';

export function OfficeAuthorButton({ folder, file }: { folder?: Schemas['Folder']; file?: Schemas['File'] }) {
  const [open, setOpen] = useState(false);
  if (file ? !file.can_update || !file.current_version_id || !/\.(docx|xlsx|pptx)$/i.test(file.name) : !folder?.can_upload) return null;
  return <><button className='button secondary' onClick={() => setOpen(true)}>{file ? '编辑 Office 内容' : '新建 Office 文件'}</button>{open && <OfficeAuthor folder={folder} file={file} close={() => setOpen(false)} />}</>;
}
function OfficeAuthor({ folder, file, close }: { folder?: Schemas['Folder']; file?: Schemas['File']; close(): void }) {
  const queue = useUploadQueue();
  const [kind, setKind] = useState<OfficeKind>('docx'), [name, setName] = useState(''), [title, setTitle] = useState(''), [content, setContent] = useState(''), [table, setTable] = useState('');
  const [basis, setBasis] = useState<Schemas['OfficeRead']>(), [values, setValues] = useState<Record<string, Schemas['OfficeCell']>>({}), [page, setPage] = useState(0);
  const [artifact, setArtifact] = useState<Schemas['OfficeArtifact']>(), [confirmed, setConfirmed] = useState(false), [joined, setJoined] = useState(false), [createKey] = useState(() => crypto.randomUUID());
  const read = useMutation({ mutationFn: async () => { await ensureDownloadSession(); return api<Schemas['OfficeRead']>('/files/' + file!.id + '/versions/' + file!.current_version_id + '/office'); }, onSuccess: value => { setBasis(value); setValues({}); setPage(0); setArtifact(undefined); } });
  const render = useMutation({ mutationFn: async () => {
    await ensureDownloadSession();
    const body = file ? { expected_version: basis!.file.version, base_version_id: basis!.version_id, source_sha256: basis!.source_sha256, workspace_sha256: basis!.workspace.sha256, changes: officeChanges(basis!.workspace, values) } : { expected_version: folder!.version, name, document: officeDocument(kind, title, content, table) };
    return checkedOfficeArtifact(await api<Schemas['OfficeArtifact']>((file ? '/files/' + file.id : '/folders/' + folder!.id) + '/office-render', { method: 'POST', body }));
  }, onSuccess: value => { setArtifact(value); setConfirmed(false); } });
  const submit = useMutation({ mutationFn: async () => {
    const a = checkedOfficeArtifact(artifact!); const bytes = Uint8Array.from(atob(a.content_base64), ch => ch.charCodeAt(0));
    const source = new File([bytes], a.name, { type: a.media_type, lastModified: 0 });
    if (await sourceFingerprint(source, undefined, a.upload.source_fingerprint) !== a.upload.source_fingerprint) throw new Error('生成文件的完整摘要不一致。');
    await queue.create(source, a.upload, createKey);
  }, onSuccess: () => setJoined(true) });
  const busy = read.isPending || render.isPending || submit.isPending;
  const units = basis?.workspace.units ?? [];
  return <Dialog title={file ? '编辑 Office 内容' : '新建 Office 文件'} onClose={close} busy={busy}>
    <p className='muted'>生成独立候选文件，核对后加入上传队列。扫描和完整摘要校验通过后，再人工提交正式版本。</p>
    <ErrorNotice error={read.error ?? render.error ?? submit.error} />
    {joined ? <p role='status'>已加入上传队列。<Link to='/uploads'>查看传输、扫描和正式提交</Link></p> : artifact ? <>
      <h3>{artifact.name}</h3><p>{artifact.size_bytes} 字节 · {artifact.workspace.units.length} 个内容片段</p>
      {artifact.workspace.warnings.map(text => <p className='muted' key={text}>{text}</p>)}
      {artifact.workspace.units.slice(0, 10).map(unit => <p key={unit.id}><strong>{unit.group} · {unit.label}</strong> {unit.value}</p>)}
      <p className='muted'>预览展示前 10 个片段；文件 SHA-256：<code>{artifact.sha256}</code></p>
      <label className='checkbox-label'><input type='checkbox' checked={confirmed} onChange={e => setConfirmed(e.target.checked)} />我已核对内容和目标，确认加入上传队列。</label>
      <div className='dialog-actions'><button className='button secondary' disabled={busy} onClick={() => { setArtifact(undefined); setConfirmed(false); }}>返回修改</button><button className='button primary' disabled={busy || !confirmed} onClick={() => submit.mutate()}>加入上传队列</button></div>
    </> : <>
      {file ? <><p>编辑目标：{file.name} · 当前版本内容</p><button className='button secondary' disabled={busy} onClick={() => read.mutate()}>{basis ? '重新读取依据（清空当前修改）' : '读取可编辑内容'}</button>
        {basis?.workspace.warnings.map(text => <p key={text} className='muted'>{text}</p>)}
        {units.slice(page * 20, page * 20 + 20).map(unit => <Field key={unit.id} label={unit.group + ' · ' + unit.label} hint={unit.read_only ? '此片段保持只读' : unit.kind === 'formula' ? '安全公式子集；由原编辑器重算' : undefined}>
          <textarea rows={2} maxLength={8192} disabled={busy || unit.read_only} value={values[unit.id]?.value ?? unit.value} onChange={e => setValues(current => ({ ...current, [unit.id]: { kind: unit.kind, value: e.target.value } }))} />
        </Field>)}
        {units.length > 20 && <div className='dialog-actions'><button className='button secondary' disabled={page === 0 || busy} onClick={() => setPage(p => p - 1)}>上一页</button><span>{page + 1} / {Math.ceil(units.length / 20)}</span><button className='button secondary' disabled={(page + 1) * 20 >= units.length || busy} onClick={() => setPage(p => p + 1)}>下一页</button></div>}
      </> : <>
        <Field label='文件类型'><select value={kind} disabled={busy} onChange={e => setKind(e.target.value as OfficeKind)}>{Object.entries(officeKinds).map(([key, label]) => <option key={key} value={key}>{label}</option>)}</select></Field>
        <Field label='文件名称'><input value={name} maxLength={180} disabled={busy} onChange={e => setName(e.target.value)} /></Field>
        <Field label='文档标题'><input value={title} maxLength={256} disabled={busy} onChange={e => setTitle(e.target.value)} /></Field>
        <Field label='文档内容' hint={officeHint[kind]}><textarea rows={8} maxLength={100000} value={content} disabled={busy} onChange={e => setContent(e.target.value)} /></Field>
        {kind === 'docx' && <Field label='可选表格' hint='从表格粘贴，Tab 分列、换行分行；各行列数一致。'><textarea rows={4} value={table} disabled={busy} maxLength={50000} onChange={e => setTable(e.target.value)} /></Field>}
      </>}
      <div className='dialog-actions'><button className='button secondary' disabled={busy} onClick={close}>关闭</button><button className='button primary' disabled={busy || (file ? !basis || !officeChanges(basis.workspace, values).length : !name.trim())} onClick={() => render.mutate()}>生成并核对候选文件</button></div>
    </>}
  </Dialog>;
}
