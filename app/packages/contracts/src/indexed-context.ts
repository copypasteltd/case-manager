import type { components } from './api';

type ContextView = components['schemas']['ConversationContextView'];
const hash = /^[a-f0-9]{64}$/;

/** The optional indexed projection must refer to the same exact formal file. */
export function assertIndexedContext(view: ContextView): void {
  const index = view.text_index;
  if (index === undefined) return;
  const fail = () => { throw new Error('文件正文提取证据不完整或与准确版本不一致，请重新核对。'); };
  if (!index || view.type !== 'file' || !view.version_id || index.state !== 'ready'
    || typeof index.extractor !== 'string' || !/^[a-z][a-z0-9-]{0,63}$/.test(index.extractor)
    || typeof index.source_sha256 !== 'string' || !hash.test(index.source_sha256)
    || typeof index.text_sha256 !== 'string' || !hash.test(index.text_sha256)
    || typeof index.indexed_at !== 'string' || !Number.isFinite(Date.parse(index.indexed_at))
    || index.error_code !== null || !Number.isSafeInteger(index.total_characters)
    || index.total_characters === null || index.total_characters < 0 || index.total_characters > 2 ** 21
    || view.total_chars !== index.total_characters || !view.untrusted_data
    || !Array.isArray(view.evidence) || !view.evidence.some(file => file.file_id === view.id
      && file.version_id === view.version_id && file.sha256 === index.source_sha256 && file.scan_state === 'clean')) fail();
  if (!Number.isSafeInteger(view.offset) || view.offset < 0 || typeof view.text !== 'string') fail();
  const end = view.offset + [...view.text].length;
  if (end > view.total_chars || (end < view.total_chars ? view.next_offset !== end : view.next_offset !== null)) fail();
}
