import { Text, View } from '@tarojs/components';
import { useEffect, useRef, useState } from 'react';
import { checkedSearch, checkedFileText, searchPath, fileTextPath, resourceNames, textStates, extractionNotice, nativePageNotice, documentTextNotice, type SearchHit, type SearchType } from '@copypaste/contracts/search';
import { client } from '../../platform/taro';
import { Scope, MobileError } from '../../platform/session';
import { usePage, go } from '../../platform/page';
import { Input } from '../../platform/components';
import { Action, Busy, Empty, Field, Notice, Page, Section, date } from '../../ui';
import { SelectField } from '../business/controls';

function useResource<T>(path: string | null, parent: Scope, check: (value: unknown) => T) {
  const validate = useRef(check); validate.current = check;
  const [value, setValue] = useState<T>(), [error, setError] = useState<unknown>(), [loading, setLoading] = useState(!!path);
  useEffect(() => { const scope = new Scope(), unbind = parent.own(() => scope.close()); setValue(undefined); setError(undefined); setLoading(!!path);
    if (path) void client.request(path, { scope }).then(v => { if (!scope.closed) setValue(validate.current(v)); }).catch(e => { if (!scope.closed) setError(e instanceof MobileError ? e : new MobileError('SEARCH_READ_FAILED', e instanceof Error ? e.message : '资料暂时无法读取。')); }).finally(() => { if (!scope.closed) setLoading(false); });
    return () => { scope.close(); unbind(); };
  }, [path, parent]); return { value, error, loading };
}
export default function SearchPage() {
  const page = usePage(async () => null);
  return <Page title="搜索工作资料" eyebrow="FIND THE RIGHT CONTEXT"><Notice error={page.error} />{page.loading ? <Busy /> : page.result && page.scope && <Workspace key={page.result.me.user.id + '/' + page.result.me.authorization_version} scope={page.scope} />}</Page>;
}
function Workspace({ scope }: { scope: Scope }) {
  const [draft, setDraft] = useState(''), [type, setType] = useState<SearchType>(''), [applied, setApplied] = useState<{ q: string; type: SearchType }>(), [cursors, setCursors] = useState(['']), [error, setError] = useState<unknown>(), [preview, setPreview] = useState<SearchHit>();
  const path = applied ? searchPath(applied.q, applied.type, cursors[cursors.length - 1]) : null;
  const results = useResource(path, scope, checkedSearch);
  function search() { try { searchPath(draft, type); setError(undefined); setPreview(undefined); setCursors(['']); setApplied({ q: draft.trim(), type }); } catch (e) { setError(new MobileError('INVALID_SEARCH', e instanceof Error ? e.message : '请输入搜索词句。')); } }
  return <><Text className="cp-muted">查找项目、任务、知识记录、目录和当前版本文件正文。</Text><Section title="输入名称或正文词句"><Field label="搜索工作资料"><Input ariaLabel="搜索工作资料" className="cp-input" value={draft} maxlength={200} onInput={e => setDraft(e.detail.value)} placeholder="例如：项目范围、交付约定" onConfirm={search} /></Field><SelectField disabled={false} label="资源类型" value={type} options={[['', '所有资料'], ...Object.entries(resourceNames).map(([value, label]) => [value, label] as const)]} onChange={v => setType(v as SearchType)} /><Action onClick={search}>搜索资料</Action></Section><Notice error={error ?? results.error} />
    {results.loading ? <Busy /> : !applied ? <Empty>从一个词句开始。正文尚未提取的文件也可以按名称查找。</Empty> : results.value && <Section title={'“' + applied.q + '”的搜索结果'}>{results.value.items.length ? results.value.items.map(h => <View className="cp-search-hit" key={h.resource.type + '/' + h.resource.id}><Text className="cp-muted cp-small">{resourceNames[h.resource.type as keyof typeof resourceNames]} · {h.match_types.map(m => ({ name: '名称', body: '正文', tags: '标签' })[m]).join('／')}命中</Text><Text className="cp-section-title">{h.title}</Text>{h.text_extractor === 'document-ocr-v1' && <Text className="cp-notice cp-paragraph">{documentTextNotice}</Text>}{!!h.snippet && <Text className="cp-search-snippet">{h.snippet.replace(/\s+/gu,' ')}</Text>}<Text className="cp-muted cp-small">更新 {date(h.updated_at)}{h.text_state && ' · ' + textStates[h.text_state]}</Text>{h.indexed_at && <Text className="cp-muted cp-small">提取 {date(h.indexed_at)}</Text>}<Action secondary onClick={() => go(h.resource.type, h.resource.id)}>打开{resourceNames[h.resource.type as keyof typeof resourceNames]}</Action>{h.resource.type === 'file' && h.resource.version_id && <Action secondary onClick={() => setPreview(h)}>查阅此版本正文</Action>}</View>) : <Empty>没有找到可见资料。可尝试其他词句或资源类型。</Empty>}{cursors.length > 1 && <Action secondary onClick={() => { setPreview(undefined); setCursors(old => old.slice(0, -1)); }}>上一页结果</Action>}{results.value.next_cursor && <Action secondary onClick={() => { setPreview(undefined); setCursors(old => [...old, results.value!.next_cursor!]); }}>下一页结果</Action>}</Section>}
    {preview?.resource.version_id && <Preview key={preview.resource.id + preview.resource.version_id} hit={preview} scope={scope} close={() => setPreview(undefined)} />}</>;
}
function Preview({ hit, scope, close }: { hit: SearchHit; scope: Scope; close(): void }) {
  const file = hit.resource.id, version = hit.resource.version_id!, [offsets, setOffsets] = useState([0]), offset = offsets[offsets.length - 1];
  const text = useResource(fileTextPath(file, version, offset), scope, v => checkedFileText(v, file, version, offset));
  return <Section title={hit.title + ' · 正文'}><Text className="cp-muted cp-small">固定版本 {version}</Text><Notice error={text.error} />{text.loading ? <Busy /> : text.value && <>{text.value.state === 'ready' ? <><Text className="cp-muted cp-small">提取 {date(text.value.indexed_at)} · 字符 {offset + 1}–{offset + [...text.value.text].length}／{text.value.total_characters}</Text>{text.value.extractor !== 'text-v1' && <Text className="cp-notice cp-paragraph">{extractionNotice(text.value.extractor)} {nativePageNotice(text.value.native_evidence)}</Text>}<Text className="cp-search-snippet">{text.value.text || '此版本没有可显示的正文。'}</Text></> : <Text>{textStates[text.value.state]}。可打开文件详情查阅原件。</Text>}{offsets.length > 1 && <Action secondary onClick={() => setOffsets(old => old.slice(0, -1))}>上一段正文</Action>}{text.value.next_offset !== null && <Action secondary onClick={() => setOffsets(old => [...old, text.value!.next_offset!])}>下一段正文</Action>}</>}<Action secondary onClick={close}>关闭正文</Action></Section>;
}
