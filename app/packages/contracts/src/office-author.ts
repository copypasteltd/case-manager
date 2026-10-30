import type { components } from './api';
type S = components['schemas'];
export type OfficeKind = S['OfficeDocument']['kind'];
export const officeKinds = { docx: 'Word 文档', xlsx: 'Excel 工作簿', pptx: 'PowerPoint 演示' } as const;
export const officeHint = { docx: '正文每行一个段落；可选表格用 Tab 分列、换行分行。', xlsx: '每行一条记录，用 Tab 分列。默认按文字保存；数字写 n:1200，公式写 f:=SUM(B1:B2)，布尔值写 b:true。', pptx: '每张幻灯片第一行是标题，后续行是正文；用独立一行 --- 分隔幻灯片。' } as const;
export function officeDocument(kind: OfficeKind, title: string, content: string, table = ''): S['OfficeDocument'] {
  if (!content.trim() && !table.trim()) throw new Error('请填写需要写入的内容。');
  if (content.length + table.length > 100_000) throw new Error('内容过长，请分成较小文档。');
  const lines = (text: string) => text.replace(/\r\n?/g, '\n').split('\n');
  if (kind === 'docx') return { kind, title, paragraphs: content ? lines(content) : [], table: table ? lines(table).map(line => line.split('\t')) : [] };
  if (kind === 'xlsx') return { kind, title, sheets: [{ name: '工作表1', rows: lines(content).map(line => line.split('\t').map(value => value.startsWith('n:') ? { kind: 'number', value: value.slice(2) } : value.startsWith('f:') ? { kind: 'formula', value: value.slice(2) } : value.startsWith('b:') ? { kind: 'boolean', value: value.slice(2) } : { kind: 'text', value: value.startsWith('t:') ? value.slice(2) : value })) }] };
  return { kind, title, slides: content.replace(/\r\n?/g, '\n').split(/^---\s*$/m).map(text => { const [heading, ...body] = lines(text.trim()); return { title: heading, lines: body }; }) };
}
export function officeChanges(workspace: S['OfficeWorkspace'], values: Record<string, S['OfficeCell']>): S['OfficeChange'][] {
  return workspace.units.filter(unit => !unit.read_only && values[unit.id] && (values[unit.id].kind !== unit.kind || values[unit.id].value !== unit.value)).map(unit => ({ id: unit.id, ...values[unit.id] }));
}
export function checkedOfficeArtifact(a: S['OfficeArtifact']): S['OfficeArtifact'] {
  const size = Number(a.size_bytes);
  if (a.workspace.adapter !== 'office-author-v1' || !Number.isSafeInteger(size) || size < 1 || size > 12 * 1024 * 1024 || String(size) !== a.size_bytes ||
    !/^[a-f0-9]{64}$/.test(a.sha256) || a.upload.source_fingerprint !== 'office-artifact-v1:' + a.sha256 || a.upload.size_bytes !== a.size_bytes || a.upload.name !== a.name || a.upload.media_type !== a.media_type ||
    a.content_base64.length !== Math.ceil(size / 3) * 4 || !/^[A-Za-z0-9+/]*={0,2}$/.test(a.content_base64) || (size % 3 === 0 ? a.content_base64.endsWith('=') : size % 3 === 1 ? !a.content_base64.endsWith('==') : !a.content_base64.endsWith('=') || a.content_base64.endsWith('=='))) throw new Error('生成文件与上传依据不一致，请重新生成。');
  return a;
}
