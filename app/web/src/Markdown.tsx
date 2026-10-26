import { Fragment, type ReactNode } from 'react';

function inline(text: string): ReactNode[] {
  const pattern = /(`[^`\n]+`|\*\*[^*\n]+\*\*|\*[^*\n]+\*|\[[^\]\n]+\]\([^)\s]+\))/g;
  const output: ReactNode[] = []; let previous = 0;
  for (const match of text.matchAll(pattern)) {
    const index = match.index ?? 0, value = match[0];
    if (index > previous) output.push(text.slice(previous, index));
    if (value.startsWith('[') && index > 0 && text[index - 1] === '!') output.push(value);
    else if (value.startsWith('`')) output.push(<code key={index}>{value.slice(1, -1)}</code>);
    else if (value.startsWith('**')) output.push(<strong key={index}>{value.slice(2, -2)}</strong>);
    else if (value.startsWith('*')) output.push(<em key={index}>{value.slice(1, -1)}</em>);
    else {
      const link = /^\[([^\]]+)\]\(([^)]+)\)$/.exec(value);
      let safe = false;
      if (link) { try { const url = new URL(link[2]); safe = url.protocol === 'https:' || url.protocol === 'http:'; } catch {} }
      output.push(safe && link ? <a key={index} href={link[2]} target="_blank" rel="noopener noreferrer">{link[1]}</a> : value);
    }
    previous = index + value.length;
  }
  if (previous < text.length) output.push(text.slice(previous));
  return output;
}
// Uploaded HTML remains text; only these Markdown constructs create React nodes.
export default function Markdown({ body }: { body: string }) {
  const lines = body.replace(/\r\n/g, '\n').split('\n'), blocks: ReactNode[] = [];
  for (let index = 0; index < lines.length; index++) {
    const line = lines[index], key = index;
    if (line.startsWith('```')) {
      const code: string[] = [];
      while (++index < lines.length && !lines[index].startsWith('```')) code.push(lines[index]);
      blocks.push(<pre key={key}><code>{code.join('\n')}</code></pre>); continue;
    }
    if (!line.trim()) continue;
    const heading = /^(#{1,3})\s+(.+)$/.exec(line);
    if (heading) {
      const text = inline(heading[2]);
      blocks.push(heading[1].length === 1 ? <h2 key={key}>{text}</h2> : heading[1].length === 2 ? <h3 key={key}>{text}</h3> : <h4 key={key}>{text}</h4>); continue;
    }
    if (/^[-*]\s+/.test(line)) {
      const list: ReactNode[] = [];
      do { list.push(<li key={index}>{inline(lines[index].replace(/^[-*]\s+/, ''))}</li>); index++; } while (index < lines.length && /^[-*]\s+/.test(lines[index]));
      index--; blocks.push(<ul key={key}>{list}</ul>); continue;
    }
    if (/^>\s?/.test(line)) { blocks.push(<blockquote key={key}>{inline(line.replace(/^>\s?/, ''))}</blockquote>); continue; }
    const paragraph: string[] = [line];
    while (index + 1 < lines.length && lines[index + 1].trim() && !/^(#{1,3}\s|[-*]\s|>|```)/.test(lines[index + 1])) paragraph.push(lines[++index]);
    blocks.push(<p key={key}>{paragraph.map((text, i) => <Fragment key={i}>{i > 0 && <br />}{inline(text)}</Fragment>)}</p>);
  }
  return <div className="markdown-content">{blocks}</div>;
}
