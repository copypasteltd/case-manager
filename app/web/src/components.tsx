import { useEffect, useRef, useId, cloneElement, isValidElement, type ReactNode } from 'react';
import { LoaderCircle, X, AlertCircle } from 'lucide-react';
import { ApiError } from './api';

export function Busy({ label = '正在读取工作空间' }: { label?: string }) { return <div className="busy" role="status"><LoaderCircle size={20} className="spin" /><span>{label}</span></div>; }
export function ErrorNotice({ error }: { error: unknown }) {
  if (!error) return null;
  const message = error instanceof Error ? error.message : '操作未能完成，请稍后重试。';
  return <div className="notice danger" role="alert"><AlertCircle size={18} /><div><strong>{message}</strong>{error instanceof ApiError && <>{error.details?.filter(detail => detail.safe_message !== message).map((detail, index) => <p key={index}>{detail.safe_message}</p>)}{error.requestID && <small>请求编号 {error.requestID}</small>}</>}</div></div>;
}
const stateNames: Record<string, string> = { queued: '等待复制', copying: '复制中', verifying: '校验中', verified: '已校验', retry_wait: '等待重试', failed: '失败', missing: '缺失', replica_unconfirmed: '校验证据待核对', resource_attention: '资源待核对', resource_record: '资源进展记录', draft: '草稿', published: '已发布', active: '进行中', on_hold: '暂缓', closing: '结项核查', archived: '已归档', cancelled: '已取消', todo: '待开始', in_progress: '进行中', in_review: '待验收', done: '已完成', blocked: '受阻', open: '待处理', achieved: '已达成', missed: '已错过', mitigating: '应对中', closed: '已关闭', realized: '已转问题', investigating: '调查中', resolved: '已解决' };
export function State({ state }: { state: string }) { const tone = state === 'done' || state === 'active' ? 'success' : state === 'blocked' || state === 'on_hold' ? 'warning' : state === 'in_review' || state === 'in_progress' || state === 'closing' ? 'info' : 'neutral'; return <span className={'badge ' + tone}><i />{stateNames[state] ?? state}</span>; }
export function Empty({ title, detail, action }: { title: string; detail: string; action?: ReactNode }) { return <div className="empty"><span className="empty-symbol" aria-hidden="true">↗</span><h3>{title}</h3><p>{detail}</p>{action}</div>; }
export function Dialog({ title, children, onClose, busy = false }: { title: string; children: ReactNode; onClose: () => void; busy?: boolean }) {
  const titleID = useId();
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const dialog = ref.current; dialog?.showModal();
    return () => {
      dialog?.close();
      const parent = [...document.querySelectorAll<HTMLDialogElement>('dialog[open]')].filter(item => item !== dialog).at(-1);
      const usable = previous?.isConnected && !previous.matches(':disabled') && previous.getClientRects().length > 0;
      const target = parent
        ? usable && parent.contains(previous) ? previous : parent.querySelector<HTMLElement>('button:not([disabled])') ?? parent
        : usable ? previous : document.getElementById('main-content');
      target?.focus({ preventScroll: true });
    };
  }, []);
  return <dialog ref={ref} className="dialog" aria-labelledby={titleID} onCancel={event => { event.preventDefault(); if (!busy) onClose(); }}><header><h2 id={titleID}>{title}</h2><button type="button" className="icon-button" aria-label="关闭弹窗" onClick={onClose} disabled={busy}><X size={20} /></button></header>{children}</dialog>;
}
export function Field({ label, children, hint }: { label: string; children: ReactNode; hint?: string }) {
  const generatedID = useId();
  const id = isValidElement<{ id?: string }>(children) ? children.props.id ?? generatedID : generatedID;
  const control = isValidElement<{ id?: string; 'aria-describedby'?: string }>(children) ? cloneElement(children, { id, 'aria-describedby': hint ? id + '-hint' : undefined }) : children;
  return <div className="field"><label htmlFor={id}>{label}</label>{control}{hint && <small id={id + '-hint'}>{hint}</small>}</div>;
}
export const classificationNames: Record<string, string> = { internal: '公司内部', project_restricted: '项目受限', confidential: '严格受限' };
export const date = (value?: string | null) => value ? new Intl.DateTimeFormat('zh-HK', { timeZone: 'Asia/Hong_Kong', year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date(value)) : '未设置';
