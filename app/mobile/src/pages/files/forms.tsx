import { Text, View } from '@tarojs/components';
import { useRef, useState } from 'react';
import { useCollection } from '../../platform/collection';
import { CommandNotice, DraftNotice, useCommand } from '../../platform/command';
import { useFormDraft } from '../../platform/form';
import { canChooseFolder, classifications, createFolderPayload, fileStatePayload, fileStateReceipt, folderCreateReceipt, movePayload, moveReceipt, type FileMove, type FileResource, type Folder, type ResourceKind, type StoredFile } from '../../platform/file-input';
import { MobileError, type Schemas } from '../../platform/session';
import { Action, Busy, Check, Empty, Fact, Notice, Row, Section } from '../../ui';
import { SelectField, TextField } from '../business/controls';

type Mode = 'create' | 'move' | 'trash' | 'restore';
type Preview = { body: Schemas['FolderCreate'] | FileMove | Schemas['Transition'] | Schemas['RestoreFile']; target?: Folder; targetPath?: string };
const problem = (cause: unknown) => new MobileError('INPUT_REQUIRED', cause instanceof Error ? cause.message : '请重新核对资料。');

export function FolderPicker({ kind, source, onChoose, onClose }: {
  kind: ResourceKind; source: FileResource; onChoose(folder: Folder, path: string): void; onClose(): void;
}) {
  const [path, setPath] = useState<Folder[]>([]), current = path.at(-1);
  const entries = useCollection<Folder>('/folders?limit=20' + (current ? '&parent_id=' + current.id : '') + (source.project_id ? '&project_id=' + source.project_id : ''));
  const folders = entries.items.filter(folder => folder.project_id === source.project_id && (kind !== 'folder' || folder.id !== source.id));
  const location = path.map(folder => folder.name).join(' / ');
  return <Section title='选择目标目录'><View className='cp-pad'>
    <Text className='cp-paragraph'>{location || (source.project_id ? '本项目资料根目录' : '公司资料根目录')}</Text>
    <Text className='cp-muted cp-small'>仅可选同项目、同分类且当前允许新建的目录。</Text>
    <Notice error={entries.error} />
    {current && <><Action secondary onClick={() => setPath(old => old.slice(0, -1))}>返回上一级候选目录</Action>
      <Action disabled={!canChooseFolder(kind, source, current)} onClick={() => onChoose(current, location)}>选用当前目录：{current.name}</Action></>}
    {entries.loading ? <Busy /> : folders.length ? folders.map(folder => <View key={folder.id}>
      <Row title={folder.name} subtitle={classifications[folder.classification as keyof typeof classifications]} onClick={() => { if (path.length < 32) setPath(old => [...old, folder]); }} />
      <Action secondary disabled={!canChooseFolder(kind, source, folder)} onClick={() => onChoose(folder, [...path.map(item => item.name), folder.name].join(' / '))}>选用目录：{folder.name}</Action>
    </View>) : <Empty>这里没有当前可见的子目录。</Empty>}
    {path.length >= 32 && <Text className='cp-notice'>已到当前浏览深度上限，请返回上级目录核对。</Text>}
    {entries.next && <Action secondary loading={entries.moreBusy} onClick={() => void entries.more()}>加载更多候选目录</Action>}
    <Action secondary onClick={onClose}>返回整理表单，保留输入</Action>
  </View></Section>;
}

export function FileEditor({ mode, kind = 'folder', item, parent, me, onCommitted, onClose }: {
  mode: Mode; kind?: ResourceKind; item?: FileResource; parent?: Folder; me: Schemas['Me'];
  onCommitted(value: FileResource): void; onClose(): void;
}) {
  const identity = mode === 'create' ? parent?.id ?? 'company-files-root' : item!.id;
  const draft = useFormDraft(me, { kind: mode === 'create' ? 'folder-create' : kind === 'folder' ? 'folder-organize' : mode === 'move' ? 'file-organize' : 'file-state', id: identity, version: mode === 'create' ? 1 : item!.version }, mode === 'create' ? parent?.classification ?? 'internal' : mode, { name: mode === 'create' ? '' : item!.name });
  const [error, setError] = useState<unknown>(), [target, setTarget] = useState<Folder>(), [targetPath, setTargetPath] = useState(''), [choosing, setChoosing] = useState(false);
  const [preview, setPreview] = useState<Preview>(), [confirmed, setConfirmed] = useState(false);
  const pending = useRef<Preview>();
  const command = useCommand<FileResource>(value => { draft.forget(); onCommitted(value); }, draft.binding, value => {
    const request = pending.current;
    if (!request) return false;
    if (mode === 'create') return folderCreateReceipt(value, request.body as Schemas['FolderCreate'], me.user.id, parent);
    if (mode === 'move') return moveReceipt(value, kind, item!, request.body as FileMove);
    return fileStateReceipt(value, item as StoredFile, mode);
  });
  const disabled = command.busy || command.blocked, name = draft.fields.name ?? '', why = draft.fields.reason;
  const title = mode === 'create' ? '新建目录' : mode === 'move' ? (kind === 'folder' ? '整理目录' : '整理文件') : mode === 'trash' ? '移入回收站' : '恢复至原目录';
  function changed(change: { name?: string; reason?: string; action?: string }) {
    draft.update(change); setError(undefined); setPreview(undefined); setConfirmed(false);
  }
  function prepare() {
    try {
      const body = mode === 'create' ? createFolderPayload(name, parent?.classification ?? draft.fields.action, parent)
        : mode === 'move' ? movePayload(kind, item!, name, why, target) : fileStatePayload(item as StoredFile, mode, why);
      setError(undefined); setPreview({ body, target, targetPath }); setConfirmed(false);
    } catch (cause) { setPreview(undefined); setError(problem(cause)); }
  }
  if (choosing && item) return <FolderPicker kind={kind} source={item} onClose={() => setChoosing(false)} onChoose={(folder, path) => { setTarget(folder); setTargetPath(path); setChoosing(false); setPreview(undefined); setConfirmed(false); setError(undefined); }} />;
  return <Section title={title}><View className='cp-pad'>
    <DraftNotice restored={draft.restored} /><Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} />
    {item && <Fact label='本次准确对象'><Text>{item.name} · 修订 {item.version}</Text><Text className='cp-code' selectable>{item.id}</Text></Fact>}
    {(mode === 'create' || mode === 'move') && <TextField name='file-name' label={mode === 'create' ? '目录名称' : '新名称'} value={name} disabled={disabled} max={400} onChange={value => changed({ name: value })} hint='保留原样；大小写和全角等价名称由服务器核对重名。' />}
    {mode === 'create' && (parent ? <Fact label='新目录位置与分类'><Text>{parent.name} · {classifications[parent.classification as keyof typeof classifications]}</Text><Text className='cp-code' selectable>{parent.id}</Text></Fact>
      : <SelectField label='公司根目录分类' value={draft.fields.action} options={Object.entries(classifications)} disabled={disabled} onChange={value => changed({ action: value })} />)}
    {mode === 'move' && <><Fact label='目标位置'><Text>{target ? targetPath : '保留原位置'}</Text>{target && <Text className='cp-code' selectable>{target.id}</Text>}</Fact>
      {!(kind === 'folder' && item!.project_id && (item as Folder).parent_id === null) && <Action secondary disabled={disabled} onClick={() => setChoosing(true)}>选择目标目录</Action>}
      {target && <Action secondary disabled={disabled} onClick={() => { setTarget(undefined); setTargetPath(''); setPreview(undefined); setConfirmed(false); }}>保留原位置，仅改名称</Action>}
      <Text className='cp-muted cp-small'>项目和资料分类保持当前范围。目录不能移到自身或后代；项目资料根目录只能改名。</Text></>}
    {mode !== 'create' && <TextField name='file-reason' label='操作原因' value={why} disabled={disabled} onChange={value => changed({ reason: value })} multiline max={400} />}
    {mode === 'trash' && <Text className='cp-paragraph'>文件会移至回收站，内容和全部历史版本继续保留。此操作不会删除云端原件。</Text>}
    {mode === 'restore' && <Text className='cp-paragraph'>恢复原目录、原名称及现有正式内容；名称被占用时须先解决冲突，不会覆盖其他文件。</Text>}
    <Action secondary disabled={disabled} onClick={prepare}>核对本次{title}</Action>
    {preview && <View >
      <Fact label='将提交的准确范围'><Text>{mode === 'create' || mode === 'move' ? '名称：' + String('name' in preview.body ? preview.body.name : (preview.body as FileMove).new_name) : title}</Text>
        <Text>{mode === 'create' ? '位置：' + (parent?.name ?? '公司根目录') : mode === 'move' ? '位置：' + (preview.targetPath || '原位置') : '位置与正式内容保持原记录'}</Text>
        {mode === 'create' ? <Text>分类：{classifications[(preview.body as Schemas['FolderCreate']).classification as keyof typeof classifications]}</Text> : <Text>原因：{why}</Text>}
      </Fact>
      <Check checked={confirmed} disabled={disabled} onChange={setConfirmed}>我已核对准确对象、位置及本次操作影响</Check>
      <Action loading={command.busy} disabled={disabled || !confirmed} onClick={() => {
        pending.current = preview;
        const path = mode === 'create' ? '/folders' : '/' + (kind === 'folder' ? 'folders' : 'files') + '/' + item!.id + '/' + (mode === 'move' ? 'move' : mode);
        void command.submit(path, preview.body);
      }}>确认{title}</Action>
    </View>}
    <Action secondary disabled={command.busy} onClick={onClose}>返回资料，保留文字草稿</Action>
  </View></Section>;
}
