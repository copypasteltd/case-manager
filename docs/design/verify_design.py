"""Validate documentation links, token generation and declared contrast pairs.

Native Python standard library; no product or cloud operations.
"""
import importlib.util
import json
import re
import sys
from datetime import datetime, timezone
from pathlib import Path
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parent
DOCS = ROOT.parent
sys.dont_write_bytecode = True


def luminance(value):
    channels = [int(value[i:i + 2], 16) / 255 for i in (1, 3, 5)]
    linear = [v / 12.92 if v <= 0.04045 else ((v + 0.055) / 1.055) ** 2.4 for v in channels]
    return sum(a * b for a, b in zip(linear, (0.2126, 0.7152, 0.0722)))


def contrast(a, b):
    x, y = sorted((luminance(a), luminance(b)))
    return (y + 0.05) / (x + 0.05)


def main():
    tokens = json.loads((ROOT / 'tokens.json').read_text(encoding='utf-8'))
    failures = []
    pairs = []
    for theme, values in tokens['themes'].items():
        specs = []
        for fg in ('fg.default', 'fg.muted', 'fg.placeholder', 'fg.disabled'):
            specs.extend((fg, bg, 4.5) for bg in ('bg.canvas', 'bg.surface', 'bg.subtle'))
        specs.extend(('accent.fg', bg, 4.5) for bg in ('bg.canvas', 'bg.surface'))
        specs.append(('fg.selected', 'bg.selected', 4.5))
        specs.extend(('action.on', bg, 4.5) for bg in ('action.fill', 'action.hover', 'action.pressed'))
        specs.append(('danger.on', 'danger.fill', 4.5))
        specs.extend((f'status.{s}.fg', f'status.{s}.bg', 4.5) for s in ('success', 'warning', 'danger', 'info', 'neutral'))
        specs.extend(('border.control', bg, 3.0) for bg in ('bg.canvas', 'bg.surface', 'bg.hover', 'bg.selected'))
        specs.extend(('focus.ring', bg, 3.0) for bg in ('bg.canvas', 'bg.surface', 'bg.subtle', 'bg.selected'))
        for foreground, background, minimum in specs:
            ratio = contrast(values[foreground], values[background])
            record = dict(theme=theme, foreground=foreground, background=background, ratio=round(ratio, 6), minimum=minimum, passed=ratio >= minimum)
            pairs.append(record)
            if not record['passed']:
                failures.append(record)
    excluded = {
        'theme': 'light', 'foreground': 'accent.fg', 'background': 'bg.selected',
        'ratio': contrast(tokens['themes']['light']['accent.fg'], tokens['themes']['light']['bg.selected']),
        'reason': 'Known failing normal-text pair: use fg.selected on bg.selected instead.'
    }
    contrast_report = {'generated_at': datetime.now(timezone.utc).isoformat(), 'method': 'WCAG sRGB relative luminance; pass compares unrounded ratios',
                       'scope': 'Only listed solid color combinations; not a full WCAG page audit',
                       'required_pairs': len(pairs), 'failures': failures.copy(), 'pairs': pairs, 'excluded_pair': excluded}
    (ROOT / 'evidence' / 'contrast-report.json').write_text(json.dumps(contrast_report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    spec = importlib.util.spec_from_file_location('cp_build_tokens', ROOT / 'build_tokens.py')
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    css_equal = module.build(tokens) == (ROOT / 'tokens.css').read_text(encoding='utf-8')
    if not css_equal:
        failures.append('Generated CSS differs from JSON source')
    markdowns = sorted(DOCS.rglob('*.md'))
    link_count = 0
    root_index = (DOCS / 'README.md').read_text(encoding='utf-8')
    for file in markdowns:
        source = file.read_text(encoding='utf-8')
        if '\ufffd' in source or not source.endswith('\n'):
            failures.append('Encoding/newline: ' + str(file.relative_to(DOCS)))
        if file != DOCS / 'README.md' and file.relative_to(DOCS).as_posix() not in root_index:
            failures.append('Not indexed: ' + str(file.relative_to(DOCS)))
        for target in re.findall(r'!?\[[^\]]*\]\(([^)]+)\)', source):
            if target.startswith(('https://', 'http://', 'mailto:', '#')):
                continue
            target = unquote(target.split('#', 1)[0]).strip('<>')
            if not target:
                continue
            link_count += 1
            resolved = (file.parent / target).resolve()
            own_output = ROOT / 'evidence' / 'design-static-review.json'
            if not resolved.exists() and resolved != own_output:
                failures.append({'file': file.relative_to(DOCS).as_posix(), 'missing_link': target})
    theme_keys_equal = set(tokens['themes']['light']) == set(tokens['themes']['dark'])
    if not theme_keys_equal:
        failures.append('Theme keys differ')
    result = {'checked_at': datetime.now(timezone.utc).isoformat(), 'markdown_count': len(markdowns), 'local_links': link_count,
              'css_reproducible': css_equal, 'theme_keys': len(tokens['themes']['light']), 'theme_keys_equal': theme_keys_equal,
              'contrast_pairs': len(pairs), 'failures': failures}
    (ROOT / 'evidence' / 'design-static-review.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(result, ensure_ascii=False))
    return bool(failures)


if __name__ == '__main__':
    raise SystemExit(main())
