export type Inline = { text: string; kind: 'text' | 'strong' | 'em' | 'code' | 'link'; url?: string };
export type Block = { kind: 'heading' | 'paragraph' | 'quote' | 'code' | 'item'; level?: number; text: Inline[] };
export function safeLink(value: string): boolean {
  try { const url = new URL(value); return ['https:', 'http:'].includes(url.protocol) && !!url.hostname && !url.username && !url.password; } catch { return false; }
}
export function inline(text: string): Inline[] {
  const parts: Inline[] = []; let previous = 0;
  for (const match of text.matchAll(/(`[^`\n]+`|\*\*[^*\n]+\*\*|\*[^*\n]+\*|\[[^\]\n]+\]\([^)\s]+\))/g)) {
    const at = match.index!, value = match[0];
    if (at > previous) parts.push({ kind: 'text', text: text.slice(previous, at) });
    if (value.startsWith('[')) {
      const link = /^\[([^\]]+)\]\(([^)]+)\)$/.exec(value)!;
      parts.push(text[at - 1] !== '!' && safeLink(link[2]) ? { kind: 'link', text: link[1], url: link[2] } : { kind: 'text', text: value });
    } else if (value.startsWith('`')) parts.push({ kind: 'code', text: value.slice(1, -1) });
    else if (value.startsWith('**')) parts.push({ kind: 'strong', text: value.slice(2, -2) });
    else parts.push({ kind: 'em', text: value.slice(1, -1) });
    previous = at + value.length;
  }
  if (previous < text.length) parts.push({ kind: 'text', text: text.slice(previous) });
  return parts;
}
// Return text nodes only: never interpret HTML, execute scripts or fetch images.
export function markdown(value: string): Block[] {
  const lines = value.replace(/\r\n?/g, '\n').split('\n'), result: Block[] = [];
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (line.startsWith('```')) {
      const code: string[] = []; while (++i < lines.length && !lines[i].startsWith('```')) code.push(lines[i]);
      result.push({ kind: 'code', text: [{ kind: 'text', text: code.join('\n') }] }); continue;
    }
    if (!line.trim()) continue;
    const heading = /^(#{1,3})\s+(.+)$/.exec(line);
    if (heading) { result.push({ kind: 'heading', level: heading[1].length, text: inline(heading[2]) }); continue; }
    if (/^[-*]\s+/.test(line)) { result.push({ kind: 'item', text: inline(line.replace(/^[-*]\s+/, '')) }); continue; }
    if (/^>\s?/.test(line)) { result.push({ kind: 'quote', text: inline(line.replace(/^>\s?/, '')) }); continue; }
    const paragraph = [line];
    while (i + 1 < lines.length && lines[i + 1].trim() && !/^(#{1,3}\s|[-*]\s|>|```)/.test(lines[i + 1])) paragraph.push(lines[++i]);
    result.push({ kind: 'paragraph', text: inline(paragraph.join('\n')) });
  }
  return result;
}
