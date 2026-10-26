import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import Markdown from './Markdown';

describe('untrusted knowledge and comment content', () => {
  it('keeps HTML, executable URLs and image markup inert while rendering safe links', () => {
    const html = renderToStaticMarkup(<Markdown body={'<script>alert(1)</script>\n<img src=x onerror=alert(2)>\n[run](javascript:alert)\n[data](data:text/html,test)\n![pixel](https://invalid.example/pixel)\n[official](https://copypaste.hk/)'} />);
    expect(html).not.toMatch(/<(script|img|iframe|object)\b/i);
    expect(html).not.toMatch(/href="(?:javascript|data):/i);
    // Image Markdown must remain text, never become an automatically loaded URL.
    expect(html).not.toContain('href="https://invalid.example/pixel"');
    expect(html).toContain('&lt;script&gt;alert(1)&lt;/script&gt;');
    expect(html).toContain('href="https://copypaste.hk/" target="_blank" rel="noopener noreferrer"');
  });
  it('preserves document structure and code as escaped text', () => {
    const html = renderToStaticMarkup(<Markdown body={'# 结论\n\n- **已核验**\n- *待跟进*\n\n> 来源记录\n\n```html\n<img onerror=bad>\n```'} />);
    expect(html).toContain('<h2>结论</h2>');
    expect(html).toContain('<ul><li><strong>已核验</strong></li><li><em>待跟进</em></li></ul>');
    expect(html).toContain('<blockquote>来源记录</blockquote>');
    expect(html).toContain('<pre><code>&lt;img onerror=bad&gt;</code></pre>');
  });
});
