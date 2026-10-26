import test from 'node:test';
import assert from 'node:assert/strict';
import { inline, markdown, safeLink } from './post-markdown';

test('Markdown emits headings, paragraphs, quotes, lists and verbatim fenced code as text nodes', () => {
  const blocks = markdown('# 会议\r\n第一行\r\n第二行\r\n\r\n- 下一步\n> 引用\n```html\n<script>alert(1)</script>\n```');
  assert.deepEqual(blocks.map(b => b.kind), ['heading', 'paragraph', 'item', 'quote', 'code']);
  assert.equal(blocks[1].text[0].text, '第一行\n第二行'); assert.equal(blocks[4].text[0].text, '<script>alert(1)</script>');
});
test('raw HTML, remote images, script/data/file links and embedded credentials never become active resources', () => {
  for (const value of ['javascript:alert(1)', 'data:text/html,hello', 'file:///private.txt', 'https://user:password@example.com', '//example.com']) assert(!safeLink(value));
  assert(safeLink('https://example.com/a?q=中文')); assert(safeLink('http://127.0.0.1:8080/'));
  assert(!inline('![图片](https://example.com/image.png) <img src=x onerror=alert(1)>').some(p => p.kind === 'link'));
  assert(!inline('[执行](javascript:alert(1))').some(p => p.kind === 'link'));
});
test('formatting and explicit http(s) links preserve readable Unicode content without fetching', () => {
  const parts = inline('中文 **加粗** *斜体* `代码` [官网](https://example.com)');
  assert.deepEqual(parts.filter(p => p.kind !== 'text').map(p => p.kind), ['strong', 'em', 'code', 'link']);
  assert.equal(parts.at(-1)?.url, 'https://example.com');
});
