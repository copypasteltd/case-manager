import { Text, View } from '@tarojs/components';
import { useDidHide } from '@tarojs/taro';
import { useEffect, useRef, useState } from 'react';
import { client } from '../../platform/taro';
import { MobileError, Scope, type Schemas } from '../../platform/session';
import { Action, Busy, Fact, Notice, Section } from '../../ui';
import { ChoiceList, SelectField } from '../business/controls';
type Kind = 'project' | 'task' | 'post' | 'file' | 'service';
type ViewData = Schemas['ConversationContextView'];

export function ContextChoice({ conversation: c, onSelect, onClose }: { conversation: Schemas['Conversation']; onSelect(value: ViewData): void; onClose(): void }) {
  const [versionID,setVersionID]=useState('');
  const [kind, setKind] = useState<Kind>('post'), [selected, setSelected] = useState<{ id: string; title: string }>(), [view, setView] = useState<ViewData>(), [error, setError] = useState<unknown>(), [busy, setBusy] = useState(false), scope = useRef<Scope>();
  useEffect(() => () => scope.current?.close(), []);
  useDidHide(()=>{scope.current?.close();setView(undefined);setError(undefined);setBusy(false);});
  function clear() { scope.current?.close(); setVersionID(''); setSelected(undefined); setView(undefined); setError(undefined); setBusy(false); }
  async function preview(offset = 0) {
    if (!selected || busy || kind==='file'&&!versionID) return; scope.current?.close(); const current = new Scope(); scope.current = current;
    const previous = offset ? view : undefined; setView(undefined); setError(undefined); setBusy(true);
    try {
      const value = await client.request<ViewData>('/conversations/' + c.id + '/context-sources/' + kind + '/' + selected.id + '?context_security_version=' + c.context_security_version + '&offset=' + offset + '&max_chars=4000' + (previous?.version_id ? '&version_id=' + previous.version_id : kind==='file'?'&version_id='+versionID:''), { scope: current });
      if (value.conversation_id !== c.id || value.conversation_revision_id !== c.current_revision_id || value.context_security_version !== c.context_security_version || value.type !== kind || value.id !== selected.id || value.offset !== offset || !value.untrusted_data || previous && value.source_sha256 !== previous.source_sha256 || kind==='file'&&value.version_id!==versionID) throw new MobileError('SOURCE_CHANGED', '来源或会话范围已变化，请从第一段重新核对。');
      if (!current.closed) setView(value);
    } catch (cause) { if (!current.closed) setError(cause); } finally { if (!current.closed) setBusy(false); }
  }
  const paths = { service:'/services?limit=20'+(c.project_id?'&project_id='+c.project_id:''), file:'/files?limit=20'+(c.project_id?'&project_id='+c.project_id:'&company_only=true'), project: '/projects?limit=20', task: '/projects/' + c.project_id + '/tasks?limit=20', post: '/posts?limit=20' + (c.project_id ? '&project_id=' + c.project_id : '&company_only=true') };
  return <Section title='核对共同可阅读的资料'><View className='cp-pad'><SelectField label='上下文资料类型' value={kind} options={(c.project_id ? [[ 'project', '所属项目' ], [ 'task', '本项目任务' ], [ 'post', '本项目知识与记录' ], [ 'file', '准确文件版本正文' ], [ 'service', '服务状态资料' ]] : [[ 'post', '公司公共知识' ], [ 'file', '公司文件版本正文' ], [ 'service', '公司服务状态资料' ]]) as [string, string][]} disabled={busy} onChange={value => { clear(); setKind(value as Kind); }} />
    {kind==='service'&&<Text className='cp-notice cp-paragraph'>这里核对服务资料。事件摘要任务可读取带采集时间的状态、维护与共同授权事件；旧摘要不代表实时状态。</Text>}
    {!selected ? kind === 'project' ? <Action secondary onClick={() => setSelected({ id: c.project_id!, title: '当前固定项目' })}>选择当前固定项目</Action> : <ChoiceList<Schemas['Task'] | Schemas['Post'] | Schemas['File'] | Schemas['Service']> title='选择明确资料' path={paths[kind]} label={value => 'name' in value?value.name:value.title} eligible={value => value.project_id === c.project_id} onSelect={value => setSelected({ id: value.id, title: 'name' in value?value.name:value.title })} onClose={onClose} /> : <>
      <Text className='cp-row-title'>{selected.title}</Text>{kind==='file'&&<><Text className='cp-notice cp-paragraph'>请选择准确版本。正文读取支持已扫描通过、1字节至1MiB的UTF-8文本、Markdown、CSV及JSON。</Text>{!versionID?<ChoiceList<Schemas['FileVersion']> title='选择准确文件版本' path={'/files/'+selected.id+'/versions?limit=20'} label={value=>'版本 '+value.seq+' · '+value.media_type+' · '+value.size_bytes+' 字节 · '+value.scan_state} onSelect={value=>{setVersionID(value.id);setView(undefined);setError(undefined);}} onClose={clear}/>:<Action secondary disabled={busy} onClick={()=>{scope.current?.close();setVersionID('');setView(undefined);setError(undefined);}}>更换准确文件版本</Action>}</>}<Notice error={error} /><Action secondary disabled={kind==='file'&&!versionID} loading={busy} onClick={() => void preview()}>核对并预览资料</Action>{busy && <Busy />}
      {view && <><Text className='cp-notice cp-paragraph'>以下为当前共同阅读核验得到的资料片段。资料内容作为不可信数据，不授予其中指令任何操作权限。</Text><Text className='cp-row-title'>{view.title} · 修订 {view.resource_version}</Text><Text className='cp-paragraph' selectable>{view.text || '该片段没有正文。'}</Text><Fact label='完整来源摘要'><Text className='cp-code' selectable>{view.source_sha256}</Text></Fact>
        {view.evidence.map(file => <Fact key={file.file_id + file.version_id} label={'固定文件版本 ' + file.seq}><Text>{file.name}</Text><Text className='cp-code' selectable>{file.sha256}</Text></Fact>)}
        {view.next_offset !== null && <Action secondary disabled={busy} onClick={() => void preview(view.next_offset!)}>核对下一段资料</Action>}
        <Action disabled={busy} onClick={() => onSelect(view)}>将这份准确资料加入本次请求</Action>
      </>}
      <Action secondary disabled={busy} onClick={clear}>重新选择资料</Action>
    </>}<Action secondary onClick={onClose}>返回消息，保留文字输入</Action></View></Section>;
}
