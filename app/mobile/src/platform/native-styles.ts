import type { TextStyle, ViewStyle } from 'react-native';
import tokens from '@copypaste/design/tokens.json';

export type NativeTheme = 'light' | 'dark';
type Rule = ViewStyle & TextStyle;
export function nativeStyles(theme: NativeTheme): Record<string, Rule> {
  const c = tokens.themes[theme];
  const border = { borderColor: c['border.subtle'], borderWidth: 1 };
  const field = { ...border, borderColor: c['border.control'], borderRadius: 6, padding: 12, minHeight: 48, backgroundColor: c['bg.surface'], color: c['fg.default'], fontSize: 15 };
  const rules: Record<string, Rule> = {
    'cp-page': { padding: 16, paddingBottom: 28, backgroundColor: c['bg.canvas'], flexGrow: 1 },
    'cp-brand': { flexDirection: 'row', alignItems: 'center', gap: 8, marginBottom: 32 },
    'cp-logo': { width: 30, height: 30 },
    'cp-brand-name': { fontSize: 21, fontWeight: '700', letterSpacing: -1 },
    'cp-accent': { color: c['accent.fg'] },
    'cp-heading': { marginBottom: 18 },
    'cp-eyebrow': { fontSize: 10, letterSpacing: 1.5, color: c['fg.muted'], marginBottom: 9 },
    'cp-title': { fontSize: 27, lineHeight: 34, fontWeight: '600' },
    'cp-muted': { color: c['fg.muted'] },
    'cp-small': { fontSize: 13, lineHeight: 21 },
    'cp-paragraph': { fontSize: 15, lineHeight: 26, marginVertical: 9 },
    'cp-footer': { fontSize: 10, lineHeight: 17, color: c['fg.muted'], paddingTop: 26, paddingBottom: 5, borderTopColor: c['border.subtle'], borderTopWidth: 1, marginTop: 27, letterSpacing: .5 },
    'cp-search-hit': { paddingVertical:16, borderBottomWidth:1, borderBottomColor:c['border.subtle'], gap:12 },
    'cp-search-snippet': { fontSize:15, lineHeight:26 },
    'cp-section': { marginTop: 21 },
    'cp-section-title': { fontSize: 18, fontWeight: '600', marginBottom: 10 },
    'cp-panel': { ...border, borderRadius: 9, backgroundColor: c['bg.surface'], overflow: 'hidden' },
    'cp-pad': { padding: 13 },
    'cp-row': { flexDirection: 'row', alignItems: 'center', width: '100%', minHeight: 54, gap: 8, padding: 13, borderBottomWidth: 1, borderBottomColor: c['border.subtle'] },
    'cp-row-copy': { flex: 1, minWidth: 0 },
    'cp-row-title': { fontWeight: '600', lineHeight: 23 },
    'cp-row-arrow': { color: c['accent.fg'] },
    'cp-state': { alignSelf: 'flex-start', borderRadius: 4, paddingVertical: 3, paddingHorizontal: 5, fontSize: 12, lineHeight: 18 },
    'cp-button': { justifyContent: 'center', alignItems: 'center', minHeight: 48, width: '100%', paddingVertical: 10, paddingHorizontal: 12, fontWeight: '600', fontSize: 15, lineHeight: 23, marginTop: 11, borderRadius: 6 },
    'cp-primary': { backgroundColor: c['action.fill'], color: c['action.on'], borderWidth: 1, borderColor: c['action.fill'] },
    'cp-secondary': { backgroundColor: c['bg.surface'], borderWidth: 1, borderColor: c['border.control'], color: c['fg.default'] },
    'cp-link': { minHeight: 48, flexDirection: 'row', alignItems: 'center', alignSelf: 'flex-start', paddingVertical: 10, color: c['accent.fg'] },
    'cp-settings': { marginLeft: 'auto' },
    'cp-field': { marginVertical: 13 },
    'cp-label': { fontSize: 14, fontWeight: '600', marginBottom: 6 },
    'cp-input': field,
    'cp-textarea': { ...field, height: 110, textAlignVertical: 'top', lineHeight: 25 },
    'cp-picker': field,
    'cp-placeholder': { color: c['fg.placeholder'] },
    'cp-notice': { padding: 12, borderRadius: 6, marginVertical: 13, backgroundColor: c['status.info.bg'], color: c['status.info.fg'] },
    'cp-danger': { backgroundColor: c['status.danger.bg'], color: c['status.danger.fg'] },
    'cp-empty': { paddingVertical: 24, paddingHorizontal: 13, alignItems: 'center', color: c['fg.muted'] },
    'cp-fact': { paddingVertical: 12, paddingHorizontal: 13, borderBottomWidth: 1, borderBottomColor: c['border.subtle'] },
    'cp-fact-value': { marginTop: 5 },
    'cp-metrics': { flexDirection: 'row', gap: 9, marginVertical: 18 },
    'cp-metric': { flex: 1, minWidth: 0, paddingVertical: 11, paddingHorizontal: 8, borderTopWidth: 1, borderTopColor: c['border.subtle'] },
    'cp-metric-number': { fontSize: 29, fontWeight: '600', lineHeight: 37, marginVertical: 6 },
    'cp-code': { ...border, fontFamily: 'monospace', padding: 11, backgroundColor: c['bg.subtle'], borderRadius: 6, fontSize: 13, lineHeight: 22 },
    'cp-check': { flexDirection: 'row', alignItems: 'center', gap: 8, minHeight: 48, paddingVertical: 10 },
    'cp-choice': { flexDirection: 'row', gap: 9, flexWrap: 'wrap' },
    'cp-selected': { borderWidth: 2, borderColor: c['focus.ring'] },
    'cp-fixed-file': { minWidth: 0, paddingVertical: 11, borderBottomWidth: 1, borderBottomColor: c['border.subtle'] },
    'cp-comment': { minWidth: 0, paddingVertical: 15, borderTopWidth: 1, borderTopColor: c['border.subtle'] },
    'cp-content-text': { fontFamily: 'monospace', fontSize: 14, lineHeight: 25 },
    'cp-content-verified': { color: c['status.success.fg'], fontSize: 13, marginBottom: 12 },
    'cp-content-image': { width: '100%', height: 320 },
    'cp-editor-row': { paddingTop: 10, paddingBottom: 15, borderBottomWidth: 1, borderBottomColor: c['border.subtle'] },
    'cp-field-error': { marginTop: 6, lineHeight: 24, color: c['status.danger.fg'] },
    'cp-form-target': {},
    'cp-status-navigation': { flexDirection: 'row', flexWrap: 'wrap', gap: 8, marginVertical: 12 },
    'cp-status-proof': { borderLeftWidth: 3, borderLeftColor: c['accent.fg'], paddingVertical: 8, paddingLeft: 12, marginVertical: 12 },
    'cp-status-verified': { color: c['status.success.fg'] },
    'cp-status-text': {},
    'cp-disabled': { opacity: .55 },
  };
  for (const tone of ['success', 'warning', 'danger', 'info', 'neutral'] as const) rules['cp-state-' + tone] = { backgroundColor: c[`status.${tone}.bg`], color: c[`status.${tone}.fg`] };
  return rules;
}
export function nativeClassStyle(theme: NativeTheme, className = ''): Rule {
  const rules = nativeStyles(theme);
  return Object.assign({}, ...className.split(/\s+/).map(name => rules[name] ?? {}));
}
const inherited = ['color', 'fontFamily', 'fontSize', 'fontWeight', 'fontStyle', 'lineHeight', 'letterSpacing', 'textAlign'] as const;
export function textStyle(style: Rule): TextStyle {
  return Object.fromEntries(inherited.filter(key => style[key] !== undefined).map(key => [key, style[key]]));
}
