import { Text, View } from '@tarojs/components';
import Taro, { useRouter } from '@tarojs/taro';
import { useEffect, useRef, useState } from 'react';
import { client } from '../../platform/taro';
import { goVersion, identifier, usePage } from '../../platform/page';
import { MobileError, Scope, type Schemas } from '../../platform/session';
import { contentEligibility, contentFormat, readFixedContent } from '../../platform/content';
import { createContentSink, type ContentArtifact } from '../../platform/content-sink';
import { Action, Busy, Fact, Notice, Page, Section, State, bytes } from '../../ui';
import ContentView from './view';

function Reader({ file, version, scope }: { file: Schemas['File']; version: Schemas['FileVersion']; scope: Scope }) {
  const [artifact, setArtifact] = useState<ContentArtifact>(), [error, setError] = useState<unknown>(), [busy, setBusy] = useState(false), [progress, setProgress] = useState(0);
  const operation = useRef<Scope>(), owned = useRef<ContentArtifact>(), alive = useRef(true);
  const format = contentFormat(version), unavailable = contentEligibility(version, format);
  const clear = () => { operation.current?.close(); operation.current = undefined; const current = owned.current; owned.current = undefined; if (current) void current.dispose().catch(cause => { if (alive.current) setError(cause); }); if (alive.current) { setArtifact(undefined); setBusy(false); setProgress(0); } };
  useEffect(() => {
    alive.current = true;
    const unbind = scope.own(clear);
    return () => { alive.current = false; unbind(); clear(); };
    // This reader is keyed to the exact file/version and page scope.
  }, [scope]);
  const read = async () => {
    if (!format || unavailable || scope.closed) return;
    clear(); setError(undefined); setBusy(true);
    const current = new Scope(); operation.current = current;
    const unbind = scope.own(() => current.close());
    let sink: Awaited<ReturnType<typeof createContentSink>> | undefined;
    try {
      sink = await createContentSink(format, version.id);
      const result = await readFixedContent({ version, format, scope: current, sink, transport: {
        grant: (id, ownScope) => client.request('/file-versions/' + id + '/download', { method: 'POST', body: { disposition: 'attachment' }, key: client.operationKey(), scope: ownScope }),
        chunk: (id, offset, length, sha, ownScope) => client.request('/downloads/' + id + '/chunks?offset=' + offset + '&length=' + length, { scope: ownScope, ifRange: '"' + sha + '"' }),
      }, progress: (received) => { if (!current.closed && alive.current) setProgress(received); } });
      if (current.closed || scope.closed || !alive.current) { await result.dispose(); return; }
      owned.current = result; setArtifact(result);
    } catch (cause) { if (sink) await sink.abort(); if (!current.closed && !scope.closed && alive.current) setError(cause instanceof MobileError ? cause : new MobileError('CONTENT_READ_FAILED', '文件内容暂时无法读取，请重新核对权限后重试。')); }
    finally { unbind(); if (operation.current === current) { operation.current = undefined; if (alive.current) setBusy(false); } }
  };
  return <><Section title='这个固定版本'><Fact label={file.name}><Text>{version.original_name ?? file.name}</Text><Text className='cp-muted cp-small'>版本 {version.seq} · {version.id === file.current_version_id ? '当前版本' : '历史版本'} · {bytes(version.size_bytes)}</Text><View className='cp-choice'><State value={version.scan_state} /><State value={version.availability} /></View></Fact><Fact label='内容摘要 SHA-256'><Text className='cp-code' selectable>{version.sha256}</Text></Fact></Section>
    {unavailable ? <Text className='cp-notice'>{unavailable}</Text> : <><Text className='cp-muted cp-paragraph'>读取这个固定版本后核对实际内容。离开页面会清除临时内容；读取完成不代表已人工核实。</Text>{busy ? <><View className='cp-notice'><Text>正在读取固定内容 · {bytes(String(progress))} / {bytes(version.size_bytes)}</Text></View><Action secondary onClick={clear}>取消读取</Action></> : <Action onClick={() => void read()}>{artifact ? '重新读取这个版本' : '读取这个版本的内容'}</Action>}</>}
    <Notice error={error} />{artifact && <Section title='固定版本内容'><View className='cp-pad'><Text className='cp-content-verified'>完整内容摘要已核对</Text><ContentView artifact={artifact} /></View></Section>}
    {artifact && <Action secondary onClick={clear}>清除已读取内容</Action>}
    <Action secondary onClick={() => { void Taro.navigateBack().catch(() => goVersion(file.id, version.id)); }}>返回文件资料</Action></>;
}
export default function ContentPage() {
  const params = useRouter().params;
  const page = usePage(async scope => {
    const id = identifier(params.id), versionID = identifier(params.version);
    const file = await client.request<Schemas['File']>('/files/' + id, { scope });
    const version = await client.request<Schemas['FileVersion']>('/files/' + id + '/versions/' + versionID, { scope });
    if (version.file_id !== file.id) throw new MobileError('INVALID_RESOURCE', '文件版本与当前资料不符。');
    return { file, version };
  });
  const value = page.result?.value;
  return <Page title='固定版本阅读' eyebrow='COPYPASTE / FILES' home><Notice error={page.error} />{page.loading ? <Busy /> : value && page.scope && <Reader key={value.version.id} file={value.file} version={value.version} scope={page.scope} />}<Action secondary onClick={() => void page.reload()} loading={page.loading}>刷新权限与资料</Action></Page>;
}
