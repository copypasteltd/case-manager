import { Text, View } from '@tarojs/components';
import Taro, { useRouter } from '@tarojs/taro';
import { useState } from 'react';
import { useCollection } from '../../platform/collection';
import { CommandNotice, useCommand } from '../../platform/command';
import { browse, go, identifier, usePage } from '../../platform/page';
import { client } from '../../platform/taro';
import { MobileError, Scope, type Page as ResultPage, type Schemas } from '../../platform/session';
import { completePost, postClasses, postInput, postPayload, postReason, postReceipt, postTypes, restoreReceipt, revisionReceipt, transitionPayload, transitionReceipt, validPost, type Post, type PostInput, type Revision } from '../../platform/post-input';
import { Action, Busy, Check, Empty, Fact, Notice, Page, Row, Section, State, date } from '../../ui';
import { ChoiceList, SelectField, TextField } from '../business/controls';
import { Markdown } from './markdown';
import { SourcePicker, Sources } from './sources';
import { PostComments } from './comments';

export function newPost(projectID?: string) {
  void Taro.navigateTo({ url: '/pages/detail/index?kind=post&mode=new' + (projectID ? '&project_id=' + identifier(projectID) : '') });
}

export function KnowledgeList({ projectID }: { projectID?: string }) {
  const page = usePage(async scope => projectID ? client.request<Schemas['Project']>('/projects/' + identifier(projectID), { scope }) : null);
  return <Page title='知识与记录' home><Notice error={page.error} />{page.loading ? <Busy /> : page.result && <PostList key={page.result.me.user.id + ':' + page.result.me.authorization_version} me={page.result.me} project={page.result.value} />}
    <Action secondary disabled={page.loading} onClick={() => void page.reload()}>核对身份并重新读取</Action></Page>;
}
function PostList({ me, project }: { me: Schemas['Me']; project: Schemas['Project'] | null }) {
  const [search, setSearch] = useState(''), [query, setQuery] = useState(''), [state, setState] = useState('');
  const posts = useCollection<Post>('/posts?limit=20' + (project ? '&project_id=' + project.id : '') + (query ? '&q=' + encodeURIComponent(query) : '') + (state ? '&state=' + state : ''));
  return <>{project && <Text className='cp-notice'>{project.name} · 项目知识</Text>}
    <TextField name='post-search' label='查找知识与记录' value={search} disabled={posts.loading} onChange={setSearch} hint='按标题、摘要、正文或标签查找，支持中文和英文。' />
    <SelectField label='知识状态' value={state} disabled={posts.loading} onChange={setState} options={[[ '', '所有可见状态' ], [ 'draft', '草稿' ], [ 'published', '已发布' ], [ 'archived', '已归档' ]]} />
    <Action secondary loading={posts.loading} onClick={() => { setQuery(search.trim()); posts.reload(); }}>查找 / 刷新知识</Action>
    {me.actions.includes('post.write') && (!project || !['closing', 'archived', 'cancelled'].includes(project.state)) && <Action onClick={() => newPost(project?.id)}>新建知识记录</Action>}
    <Notice error={posts.error} />{posts.loading ? <Busy /> : posts.items.length ? <Section title='当前可见记录'>{posts.items.map(p => <Row key={p.id} title={p.title} state={p.state} subtitle={postTypes[p.type] + ' · ' + (p.summary || p.tags.join(' / ') || date(p.updated_at))} onClick={() => go('post', p.id)} />)}</Section> : !posts.error && <Empty>没有符合条件的知识记录。</Empty>}
    {posts.next && <Action secondary loading={posts.moreBusy} onClick={() => void posts.more()}>更多知识记录</Action>}</>;
}

export function KnowledgePage() {
  const params = useRouter().params, creating = params.mode === 'new';
  const page = usePage(async (scope, me) => {
    if (creating) {
      if (!me.actions.includes('post.write')) throw new MobileError('FORBIDDEN', '当前身份不能新建知识记录。', 403);
      return { post: undefined, project: params.project_id ? await client.request<Schemas['Project']>('/projects/' + identifier(params.project_id), { scope }) : undefined };
    }
    const post = await client.request<Post>('/posts/' + identifier(params.id), { scope });
    if (!validPost(post)) throw new MobileError('INVALID_RESPONSE', '知识修订资料尚未完整读取。');
    return { post, project: undefined };
  });
  return <Page title={creating ? '新建知识记录' : page.result?.value.post?.title ?? '知识记录'} home><Notice error={page.error} />
    {page.loading ? <Busy /> : page.result && (creating ? <PostEditor key={page.result.me.user.id + ':' + page.result.me.authorization_version} me={page.result.me} project={page.result.value.project} onClose={() => browse('posts', { project_id: params.project_id })} onSaved={p => { void Taro.redirectTo({ url: '/pages/detail/index?kind=post&id=' + p.id }); }} />
      : <PostDetail key={page.result.value.post!.id + ':' + page.result.value.post!.version + ':' + page.result.me.authorization_version} post={page.result.value.post!} me={page.result.me} reload={() => void page.reload()} />)}
    <Action secondary disabled={page.loading} onClick={() => void page.reload()}>重新读取最新知识资料</Action></Page>;
}

export async function corroborate(post: Post, before?: Post): Promise<boolean> {
  const scope = new Scope();
  try {
    const fresh = await client.request<Post>('/posts/' + post.id, { scope });
    if (!validPost(fresh) || fresh.version !== post.version || fresh.current_revision_id !== post.current_revision_id || fresh.content_hash !== post.content_hash
      || fresh.state !== post.state || fresh.published_revision_id !== post.published_revision_id || fresh.has_unpublished_changes !== post.has_unpublished_changes
      || JSON.stringify(fresh.source_refs) !== JSON.stringify(post.source_refs) || fresh.body !== post.body || fresh.title !== post.title || fresh.summary !== post.summary || JSON.stringify(fresh.tags) !== JSON.stringify(post.tags)) return false;
    if (fresh.can_read_history) {
      const revisions = await client.request<ResultPage<Revision>>('/posts/' + post.id + '/revisions?limit=20', { scope });
      return revisions.items.length > 0 && revisionReceipt(revisions.items[0], post, before);
    }
    return true;
  } finally { scope.close(); }
}

function PostEditor({ me, before, project, onSaved, onClose }: { me: Schemas['Me']; before?: Post; project?: Schemas['Project']; onSaved(value: Post): void; onClose(): void }) {
  const [input, setInput] = useState<PostInput>(() => postInput(before, project)), [projectName, setProjectName] = useState(project?.name ?? (before?.project_id ? '原所属项目' : '公司公共知识'));
  const [picker, setPicker] = useState(false), [preview, setPreview] = useState(false), [error, setError] = useState<unknown>();
  const cmd = useCommand<Post>(onSaved, 'knowledge-edit:' + (before?.id ?? 'new') + ':' + (before?.version ?? '1'), async value => postReceipt(value, input, me.user.id, before) && await corroborate(value, before));
  const disabled = cmd.busy || cmd.blocked;
  function change<K extends keyof PostInput>(key: K, value: PostInput[K]) { if (disabled) return; setError(undefined); setInput(old => ({ ...old, [key]: value })); }
  function save() { try { setError(undefined); const value = postPayload(input, before); void cmd.submit('/posts' + (before ? '/' + before.id : ''), value, before ? 'PATCH' : 'POST'); } catch (cause) { setError(cause); } }
  return <><Section title={before ? '编辑正文 · 新建不可变修订' : '记录资料'}><View className='cp-pad'>
    <Text className='cp-muted cp-small'>保存为正文修订；发布需要另行确认。修改已发布内容后，普通读者继续读取上次发布版本。</Text>
    <Fact label='所属范围'><Text>{projectName}</Text></Fact>
    {!before && <><Action secondary disabled={disabled} onClick={() => setPicker(!picker)}>选择所属项目</Action><Action secondary disabled={disabled} onClick={() => { change('project', ''); change('classification', 'internal'); setProjectName('公司公共知识'); setPicker(false); }}>设为公司公共知识</Action></>}
    {picker && <ChoiceList<Schemas['Project']> title='选择知识所属项目' path='/projects?limit=20' label={p => p.name} eligible={p => !['closing', 'archived', 'cancelled'].includes(p.state)} onSelect={p => { change('project', p.id); change('classification', p.classification ?? 'internal'); setProjectName(p.name); setPicker(false); }} onClose={() => setPicker(false)} />}
    <SelectField label='知识类型' value={input.type} options={Object.entries(postTypes)} onChange={v => change('type', v as Post['type'])} disabled={disabled || !!before} />
    <SelectField label='知识分类' value={input.classification} options={Object.entries(postClasses)} onChange={v => change('classification', v as Post['classification'])} disabled={disabled || !!before} />
    {before && <Text className='cp-muted cp-small'>原所属范围、知识类型与分类保持锁定；跨范围调整需走对应影响评估。</Text>}
    <TextField name='post-title' label='知识标题' value={input.title} onChange={v => change('title', v)} disabled={disabled} max={400} />
    <TextField name='post-summary' label='知识摘要' value={input.summary} onChange={v => change('summary', v)} disabled={disabled} multiline max={40000} />
    <TextField name='post-tags' label='知识标签' value={input.tags} onChange={v => change('tags', v)} disabled={disabled} max={2040} hint='中英文逗号分隔，最多20项，每项最多50字符，不能重复。' />
    <TextField name='post-body' label='Markdown 正文' value={input.body} onChange={v => change('body', v)} disabled={disabled} multiline max={40000} hint='支持标题、列表、引用、代码、加粗、斜体及 http(s) 链接；图片与 HTML 作为文字呈现。' />
    <Action secondary onClick={() => setPreview(!preview)}>{preview ? '收起正文预览' : '预览正文'}</Action>{preview && <Markdown body={input.body} />}
  </View></Section>
    <SourcePicker projectID={input.project} value={input.sources} onChange={v => change('sources', v)} disabled={disabled} />
    <Notice error={error ?? cmd.error} /><CommandNotice blocked={cmd.blocked} />
    <Action loading={cmd.busy} disabled={disabled} onClick={save}>{before ? '保存知识新修订' : '保存知识草稿'}</Action>
    <Action secondary disabled={cmd.busy} onClick={onClose}>关闭表单并核对资料</Action></>;
}

function PostDetail({ post, me, reload }: { post: Post; me: Schemas['Me']; reload(): void }) {
  const [editing, setEditing] = useState(false), [history, setHistory] = useState(false), [action, setAction] = useState<Schemas['PostTransition']['action']>();
  if (editing) return <PostEditor before={post} me={me} onSaved={reload} onClose={() => { setEditing(false); reload(); }} />;
  return <><Section title='当前呈现的知识修订'><View className='cp-pad'><View className='cp-choice'><State value={post.state} /><Text>{postTypes[post.type]} · {postClasses[post.classification]}</Text></View>
    <Text className='cp-muted cp-small'>{post.project_id ? '项目知识' : '公司公共知识'} · 更新于 {date(post.updated_at)}</Text>
    <Text className='cp-muted cp-small'>作者 {post.author_id}</Text>
    <Fact label='正文修订'><Text className='cp-code' selectable>{post.current_revision_id}</Text></Fact>
    <Fact label='已发布修订'><Text className='cp-code' selectable>{post.published_revision_id ?? '尚未发布'}</Text></Fact>
    {post.has_unpublished_changes && <Text className='cp-notice'>当前有未发布内容。保存与历史恢复不会自动发布。</Text>}
    <Text className='cp-muted'>{post.summary}</Text><Text className='cp-muted cp-small'>{post.tags.join(' / ')}</Text><Markdown body={post.body} />
    {!completePost(post) && <Text className='cp-notice'>当前附件集合可能经过权限过滤。请先核对完整依据；正文编辑与发布暂不开放，以免遗漏原附件。</Text>}
  </View></Section>
    <Section title={'知识附件 · 当前可见 ' + post.source_refs.length}><Sources sources={post.source_refs} />{!post.source_refs.length && <Empty>没有当前可见的附件。</Empty>}</Section>
    {post.can_edit && completePost(post) && <Action onClick={() => setEditing(true)}>编辑知识正文</Action>}
    {post.can_publish && <Section title='发布与状态变更'><View className='cp-pad'>
      {post.state !== 'archived' && completePost(post) && <Action onClick={() => setAction('publish')}>发布当前修订</Action>}
      {post.state === 'published' && <Action secondary onClick={() => setAction('archive')}>归档知识</Action>}
      {post.state !== 'draft' && <Action secondary onClick={() => setAction('draft')}>退回草稿</Action>}
    </View></Section>}
    {action && <Transition key={action} post={post} action={action} onDone={reload} onClose={() => setAction(undefined)} />}
    {post.can_read_history && <><Action secondary onClick={() => setHistory(!history)}>{history ? '收起知识历史' : '查看知识历史'}</Action>{history && <History post={post} onDone={reload} />}</>}
    <PostComments post={post} me={me} />
  </>;
}

const actions = { publish: '发布当前修订', archive: '归档知识', draft: '退回草稿' };
function Transition({ post, action, onDone, onClose }: { post: Post; action: Schemas['PostTransition']['action']; onDone(): void; onClose(): void }) {
  const [reason, setReason] = useState(''), [checked, setChecked] = useState(false), [error, setError] = useState<unknown>();
  const cmd = useCommand<Post>(onDone, 'knowledge-state:' + post.id + ':' + post.version, async v => transitionReceipt(v, post, action) && await corroborate(v));
  return <Section title={actions[action] + ' · 人工核对'}><View className='cp-pad'><Text className='cp-code'>{post.current_revision_id}</Text>
    <TextField name='post-reason' label='知识操作原因' value={reason} onChange={setReason} max={400} disabled={cmd.busy || cmd.blocked} />
    <Check checked={checked} onChange={setChecked} disabled={cmd.busy || cmd.blocked}>我已核对当前正文、附件引用方式和本次状态变更</Check>
    <Notice error={error ?? cmd.error} /><CommandNotice blocked={cmd.blocked} />
    <Action loading={cmd.busy} disabled={cmd.blocked || !checked} onClick={() => { try { setError(undefined); void cmd.submit('/posts/' + post.id + '/publish', transitionPayload(post, action, reason, checked)); } catch (cause) { setError(cause); } }}>确认{actions[action]}</Action>
    <Action secondary disabled={cmd.busy} onClick={onClose}>关闭并保留原状态</Action></View></Section>;
}

function History({ post, onDone }: { post: Post; onDone(): void }) {
  const history = useCollection<Revision>('/posts/' + post.id + '/revisions?limit=20'), [selected, setSelected] = useState<Revision>();
  return <Section title='不可变正文历史'><Notice error={history.error} />{history.loading ? <Busy /> : history.items.map(r => <Row key={r.id} title={'正文修订 ' + r.seq + ' · ' + r.title} subtitle={date(r.created_at) + (r.id === post.current_revision_id ? ' · 当前呈现' : '') + (r.id === post.published_revision_id ? ' · 已发布' : '')} onClick={() => setSelected(r)} />)}
    {history.next && <Action secondary loading={history.moreBusy} onClick={() => void history.more()}>更多知识历史</Action>}
    {selected && <View className='cp-pad'><Text className='cp-section-title'>历史修订 {selected.seq} · {selected.title}</Text><Text className='cp-muted'>{selected.summary}</Text><Text className='cp-muted cp-small'>{selected.tags.join(' / ')}</Text><Markdown body={selected.body} /><Sources sources={selected.source_refs} />
      {post.can_edit && <Restore key={selected.id} post={post} revision={selected} onDone={onDone} />}
      <Action secondary onClick={() => setSelected(undefined)}>收起所选历史正文</Action></View>}
  </Section>;
}
function Restore({ post, revision, onDone }: { post: Post; revision: Revision; onDone(): void }) {
  const [reason, setReason] = useState(''), [checked, setChecked] = useState(false), [error, setError] = useState<unknown>();
  const cmd = useCommand<Post>(onDone, 'knowledge-restore:' + post.id + ':' + post.version, async v => restoreReceipt(v, post, revision) && await corroborate(v, post));
  return <><Text className='cp-notice'>恢复会复制此历史内容建立新修订，保留原历史和已发布版本；需要另行发布。</Text>
    <TextField name='post-restore-reason' label='恢复知识原因' value={reason} onChange={setReason} max={400} disabled={cmd.busy || cmd.blocked} />
    <Check checked={checked} onChange={setChecked} disabled={cmd.busy || cmd.blocked}>我已核对历史正文与全部依据，确认建立新的正文修订</Check>
    <Notice error={error ?? cmd.error} /><CommandNotice blocked={cmd.blocked} />
    <Action loading={cmd.busy} disabled={cmd.blocked || !checked} onClick={() => { try { setError(undefined); if (!checked) return; const why = postReason(reason); void cmd.submit('/posts/' + post.id + '/restore', { expected_version: post.version, base_revision_id: post.current_revision_id, revision_id: revision.id, reason: why }); } catch (cause) { setError(cause); } }}>恢复为新的知识修订</Action></>;
}
