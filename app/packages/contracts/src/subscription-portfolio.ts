import { sha256 } from '@noble/hashes/sha2.js';
import { contractBodyBytes } from './contract-body';
import type { components } from './api';

type S = components['schemas'];
export type PortfolioPreview = S['AgentSubscriptionPortfolioPreview'];
const canonical = (v: unknown): string => Array.isArray(v) ? '[' + v.map(canonical).join(',') + ']' : v && typeof v === 'object' ? '{' + Object.keys(v).sort().map(k => JSON.stringify(k) + ':' + canonical((v as Record<string, unknown>)[k])).join(',') + '}' : JSON.stringify(v);
export function portfolioWindow(from: string, to: string): boolean {
  const day = (v: string) => /^\d{4}-\d{2}-\d{2}$/.test(v) && Number(v.slice(0, 4)) >= 1 && !Number.isNaN(Date.parse(v)) && new Date(v).toISOString().slice(0, 10) === v;
  return !!(day(from) && day(to) && Date.parse(to) > Date.parse(from) && Date.parse(to) - Date.parse(from) <= 366 * 86400000);
}
// Validate the exact reply against the selected conversation and dates. Content
// remains inert untrusted text; a digest is integrity evidence, not AI approval.
export function checkedPortfolio(v: PortfolioPreview, c: Pick<S['Conversation'], 'id' | 'current_revision_id' | 'context_security_version' | 'project_id'>, from: string, to: string): PortfolioPreview {
  const bad = () => { throw new Error('完整审阅与当前会话、日期或内容摘要不一致，请重新读取。'); };
  if (!portfolioWindow(from, to) || v.conversation_id !== c.id || v.conversation_revision_id !== c.current_revision_id || v.context_security_version !== c.context_security_version || v.project_id !== c.project_id || v.from !== from || v.to_exclusive !== to || !v.untrusted_data) bad();
  const bytes = contractBodyBytes(v.content_text);
  if (bytes.length > 89000 || [...sha256(bytes)].map(n => n.toString(16).padStart(2, '0')).join('') !== v.content_sha256) bad();
  const content = JSON.parse(v.content_text);
  if (content.view !== 'subscription_portfolio' || content.project_id !== c.project_id || content.captured_at !== v.captured_at || content.coverage_notice !== v.coverage_notice || content.untrusted_data !== true || content.report?.from !== from || content.report?.to_exclusive !== to || canonical(content.report?.summary) !== canonical(v.summary) || !v.summary.subscription_set_complete || v.summary.subscription_count !== v.summary.items.length || new Set(v.summary.items.map(i => i.id)).size !== v.summary.items.length || content.report.reviews.length !== v.summary.items.length) bad();
  return v;
}
export const portfolioState: Record<string, string> = { draft: '草稿', active: '使用中', paused: '已暂停', cancelled: '已取消', archived: '已归档' };
export function portfolioTextPage(v: PortfolioPreview, page: number) { return [...v.content_text].slice(page * 2400, (page + 1) * 2400).join(''); }
export function portfolioTextPages(v: PortfolioPreview) { return Math.max(1, Math.ceil([...v.content_text].length / 2400)); }
