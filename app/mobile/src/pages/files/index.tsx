import { Text, View } from '@tarojs/components';
import Taro from '@tarojs/taro';
import { useState } from 'react';
import { useCollection } from '../../platform/collection';
import { classifications, type FileResource, type Folder, type StoredFile } from '../../platform/file-input';
import { fileWorkspace, go, identifier, readVersion, usePage } from '../../platform/page';
import { MobileError, type Schemas } from '../../platform/session';
import { client } from '../../platform/taro';
import { UploadEntry } from '../../platform/upload-entry';
import { Action, Busy, Empty, Fact, Notice, Page, Row, Section, State, bytes, date } from '../../ui';
import { SelectField, TextField } from '../business/controls';
import { FileEditor } from './forms';
import { MultipartCleanup } from './cleanup';

type Workspace = { folder?: Folder; file?: StoredFile; project?: Schemas['Project'] };
type Editor = { mode: 'create' | 'move' | 'trash' | 'restore'; kind: 'folder' | 'file'; item?: FileResource };
const label = (value: FileResource) => classifications[value.classification as keyof typeof classifications] ?? value.classification;
export default function FilesPage() {
  const params = Taro.useRouter().params;
  const [editor, setEditor] = useState<Editor>(), [saved, setSaved] = useState<FileResource>();
  const page = usePage<Workspace>(async scope => {
    const project = params.project_id ? await client.request<Schemas['Project']>('/projects/' + identifier(params.project_id), { scope }) : undefined;
    const file = params.file_id ? await client.request<StoredFile>('/files/' + identifier(params.file_id), { scope }) : undefined;
    const folderID = file?.folder_id ?? (params.folder_id ? identifier(params.folder_id) : undefined);
    const folder = folderID ? await client.request<Folder>('/folders/' + folderID, { scope }) : undefined;
    if (project && folder && folder.project_id !== project.id || params.folder_id && file && file.folder_id !== identifier(params.folder_id)) throw new MobileError('INVALID_RESOURCE', '资料与指定目录或项目不一致，请从原列表重新打开。');
    return { folder, file, project };
  });
  const current = page.result?.value, me = page.result?.me;
  function committed(value: FileResource) { setEditor(undefined); setSaved(value); void page.reload(); }
  return <Page title={current?.file?.name ?? current?.folder?.name ?? '文件与资料'} home>
    <Notice error={page.error} />
    {saved && !page.error && <Section title='已核对操作结果'><Fact label='实际返回的对象与修订'><Text>{saved.name} · 修订 {saved.version}</Text><State value={saved.state} /><Text className='cp-code' selectable>{saved.id}</Text></Fact></Section>}
    {page.loading ? <Busy /> : current && me && <>
      <Action secondary onClick={() => fileWorkspace(current.file ? { folder_id: current.file.folder_id, project_id: current.project?.id } : current.folder?.parent_id ? { folder_id: current.folder.parent_id, project_id: current.project?.id } : current.project ? { project_id: current.project.id } : {}, true)}>{current.file ? '返回所在目录' : current.folder ? '返回上一级' : '所有可访问资料'}</Action>
      {current.folder && <Section title='当前目录'><Fact label='目录与资料范围'><Text>{current.folder.name} · 修订 {current.folder.version}</Text><Text>{label(current.folder)} · {current.folder.project_id ? '项目资料' : '公司资料'}</Text><Text className='cp-code' selectable>{current.folder.id}</Text></Fact>
        {current.folder.project_id && <Row title='查看所属项目' onClick={() => go('project', current.folder!.project_id!)} />}
      </Section>}
      {!current.file && current.folder && me.actions.includes('file.retention.manage') && <MultipartCleanup key={current.folder.id} folder={current.folder} me={me} />}
      {editor ? <FileEditor key={editor.mode + ':' + (editor.item?.id ?? current.folder?.id ?? 'root') + ':' + (editor.item?.version ?? 1) + ':' + me.authorization_version} {...editor} parent={editor.mode === 'create' ? current.folder : undefined} me={me} onCommitted={committed} onClose={() => setEditor(undefined)} />
        : current.file ? <FileDetails file={current.file} onEdit={setEditor} />
        : <DirectoryContents current={current} canCreate={current.folder ? current.folder.can_create : !current.project && me.actions.includes('folder.create')} onEdit={setEditor} />}
    </>}
    <Action secondary onClick={() => { setEditor(undefined); void page.reload(); }} loading={page.loading}>刷新当前资料</Action>
  </Page>;
}

function DirectoryContents({ current, canCreate, onEdit }: { current: Workspace; canCreate: boolean; onEdit(value: Editor): void }) {
  const [search, setSearch] = useState(''), [query, setQuery] = useState(''), [state, setState] = useState('active');
  const projectID = current.project?.id ?? current.folder?.project_id;
  const project = projectID ? '&project_id=' + projectID : '';
  const folders = useCollection<Folder>('/folders?limit=20' + project + (current.folder ? '&parent_id=' + current.folder.id : '') + '&q=' + encodeURIComponent(query));
  const files = useCollection<StoredFile>('/files?limit=20' + project + (current.folder ? '&folder_id=' + current.folder.id : '') + '&state=' + state + '&q=' + encodeURIComponent(query));
  return <>
    <Section title='整理当前资料'><View className='cp-pad'>
      {canCreate && <Action onClick={() => onEdit({ mode: 'create', kind: 'folder' })}>新建目录</Action>}
      {current.folder?.can_update && <Action secondary onClick={() => onEdit({ mode: 'move', kind: 'folder', item: current.folder })}>改名或移动当前目录</Action>}
      {current.folder && <UploadEntry folder={current.folder} />}
      <TextField name='files-query' label='查找当前范围的名称' value={search} onChange={setSearch} disabled={false} />
      <Action secondary onClick={() => setQuery(search.trim())}>查找目录与文件</Action>
      <SelectField label='文件状态' value={state} options={[[ 'active', '当前文件' ], [ 'trashed', '回收站文件' ]]} onChange={setState} disabled={false} />
      <Text className='cp-muted cp-small'>根入口展示当前可访问资料；进入具体目录后只列该目录内容。回收站不删除正式原件。</Text>
    </View></Section>
    <Section title='子目录'><Notice error={folders.error} />{folders.loading ? <Busy /> : folders.items.length ? folders.items.map(folder => <Row key={folder.id} title={folder.name} subtitle={label(folder) + (folder.project_id ? ' · 项目资料' : ' · 公司资料')} onClick={() => fileWorkspace({ folder_id: folder.id, project_id: current.project?.id }, true)} />) : <Empty>没有当前可见的子目录。</Empty>}
      {folders.next && <Action secondary loading={folders.moreBusy} onClick={() => void folders.more()}>加载更多子目录</Action>}
    </Section>
    <Section title={state === 'trashed' ? '回收站文件' : '当前文件'}><Notice error={files.error} />{files.loading ? <Busy /> : files.items.length ? files.items.map(file => <Row key={file.id} title={file.name} subtitle={label(file) + ' · 修订 ' + file.version} state={file.state} onClick={() => fileWorkspace({ file_id: file.id, project_id: current.project?.id }, true)} />) : <Empty>没有符合条件的文件。</Empty>}
      {files.next && <Action secondary loading={files.moreBusy} onClick={() => void files.more()}>加载更多文件</Action>}
    </Section>
  </>;
}

function FileDetails({ file, onEdit }: { file: StoredFile; onEdit(value: Editor): void }) {
  const versions = useCollection<Schemas['FileVersion']>('/files/' + file.id + '/versions?limit=20');
  return <>
    <Section title='文件记录'><Fact label='准确文件与状态'><Text>{file.name} · 修订 {file.version}</Text><State value={file.state} /><Text>{label(file)}</Text><Text className='cp-code' selectable>{file.id}</Text></Fact>
      <Fact label='保留状态'><Text>{file.legal_hold ? '存在保留锁' : '无保留锁标记；其他保留规则仍由服务器检查'}</Text></Fact>
      <View className='cp-pad'>{file.can_move && file.state === 'active' && <Action secondary onClick={() => onEdit({ mode: 'move', kind: 'file', item: file })}>改名或移动文件</Action>}
        {file.can_trash && file.state === 'active' && <Action secondary onClick={() => onEdit({ mode: 'trash', kind: 'file', item: file })}>移入回收站</Action>}
        {file.can_restore && file.state === 'trashed' && <Action onClick={() => onEdit({ mode: 'restore', kind: 'file', item: file })}>恢复至原目录</Action>}
        {file.state === 'active' && <UploadEntry file={file} />}
      </View>
    </Section>
    <Section title='全部正式内容版本'><Notice error={versions.error} />{versions.loading ? <Busy /> : versions.items.length ? versions.items.map(version => <Fact key={version.id} label={'内容版本 ' + version.seq + (version.id === file.current_version_id ? ' · 当前' : ' · 历史')}>
      <Text>{version.original_name ?? '历史原文件名未提供'}</Text><Text>{bytes(version.size_bytes)} · {date(version.created_at)}</Text>
      <View className='cp-choice'><State value={version.scan_state} /><State value={version.availability} /></View>
      <Text className='cp-muted cp-small'>SHA-256</Text><Text className='cp-code' selectable>{version.sha256}</Text>
      {version.replicas.map(replica => <Text key={replica.id}>{replica.provider === 'oss' ? 'OSS原件' : 'R2副本'} · {replica.state}</Text>)}
      <Action secondary disabled={file.state !== 'active' || version.availability !== 'available' || !['clean', 'waived'].includes(version.scan_state)} onClick={() => readVersion(file.id, version.id)}>查阅准确内容版本 {version.seq}</Action>
    </Fact>) : <Empty>此文件尚无正式内容版本。</Empty>}
      {versions.next && <Action secondary loading={versions.moreBusy} onClick={() => void versions.more()}>加载更多历史版本</Action>}
    </Section>
  </>;
}
