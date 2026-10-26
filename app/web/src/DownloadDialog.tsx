import { useEffect, useRef, useState } from 'react';
import { Download, LoaderCircle, ShieldCheck } from 'lucide-react';
import { api, ensureDownloadSession, type Schemas } from './api';
import { Dialog, ErrorNotice } from './components';
import { savePicker, streamDownload, type DownloadGrant } from './download';

export type DownloadVersion = Pick<Schemas['FileVersion'], 'id' | 'seq' | 'size_bytes' | 'sha256' | 'original_name' | 'scan_state'>;
export function DownloadDialog({ version, onClose }: { version: DownloadVersion; onClose: () => void }) {
  const [grant, setGrant] = useState<DownloadGrant | null>(null);
  const [error, setError] = useState<unknown>(null), [working, setWorking] = useState(false), [preparing, setPreparing] = useState(false);
  const [progress, setProgress] = useState(0), [complete, setComplete] = useState(false), [cancelled, setCancelled] = useState(false);
  const controller = useRef<AbortController | null>(null);
  const key = useRef(crypto.randomUUID()), permitExpiry = useRef(0);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => { mounted.current = false; controller.current?.abort(); };
  }, []);
  useEffect(() => {
    if (!working) return;
    const leaving = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = ''; };
    window.addEventListener('beforeunload', leaving);
    return () => window.removeEventListener('beforeunload', leaving);
  }, [working]);
  const size = Number(version.size_bytes);
  const picker = savePicker();
  async function obtain(signal?: AbortSignal) {
    await ensureDownloadSession(signal);
    if (permitExpiry.current && permitExpiry.current <= Date.now() + 30_000) key.current = crypto.randomUUID();
    const value = await api<DownloadGrant>('/file-versions/' + version.id + '/download',
      { method: 'POST', body: { disposition: 'attachment' }, key: key.current, signal });
    permitExpiry.current = Date.parse(value.permit_expires_at);
    return value;
  }
  async function prepare() {
    setPreparing(true); setError(null); setCancelled(false); setComplete(false);
    const active = new AbortController(); controller.current = active;
    try {
      const value = await obtain(active.signal);
      if (mounted.current) setGrant(value);
    } catch (err) { if (mounted.current && !active.signal.aborted) setError(err); }
    finally { if (mounted.current) setPreparing(false); }
  }
  async function save() {
    if (!grant || !picker) return;
    const active = new AbortController(); controller.current = active;
    setWorking(true); setError(null); setCancelled(false); setComplete(false); setProgress(0);
    try {
      // Invoke the native picker directly inside this click's user activation.
      const handle = await picker({ suggestedName: grant.filename });
      active.signal.throwIfAborted();
      const sink = await handle.createWritable();
      let updated = 0;
      await streamDownload({
        identity: version, initial: grant, renew: signal => obtain(signal), beforeRange: ensureDownloadSession, sink, signal: active.signal,
        progress: (written, total) => {
          const now = performance.now();
          if (mounted.current && (now - updated >= 250 || written === total)) { setProgress(written); updated = now; }
        },
      });
      if (mounted.current) setComplete(true);
    } catch (err) {
      if (mounted.current) {
        if (active.signal.aborted || (err instanceof DOMException && err.name === 'AbortError')) setCancelled(true);
        else setError(err);
      }
    } finally { if (mounted.current) setWorking(false); }
  }
  return <Dialog title={'下载内容版本 ' + version.seq} onClose={onClose} busy={working || preparing}>
    <div className="download-content">
      <p><strong>{version.original_name ?? '文件内容'}</strong></p>
      <p className="muted">{BigInt(version.size_bytes).toLocaleString('zh-HK')} 字节 · 固定内容版本 {version.seq}</p>
      {version.scan_state === 'waived' && <p className="notice warning">此版本经扫描例外审核放行，尚未取得完整扫描通过结论。请按公司约定查阅。</p>}
      <ErrorNotice error={error} />
      {grant && <div className="notice neutral"><ShieldCheck size={18} /><span>{grant.mode === 'authenticated_proxy'
        ? '此文件采用受控下载，每次读取都会检查当前登录和访问权限。'
        : '普通资料通过短时授权直接读取。已发出的读取授权在到期前可能继续有效。'}</span></div>}
      {!picker && <p className="notice warning">当前浏览器不支持分段保存。请使用支持文件保存接口的桌面版 Chrome 或 Edge。</p>}
      {grant && picker && !complete && <p className="muted">请确认本机有足够空间，并保持此页开启。切换页面、取消或读取失败，会放弃本次未完成的保存。</p>}
      {working && <div className="download-progress" role="status"><progress aria-label="下载进度" value={progress} max={Math.max(size, 1)} /><span>{size ? Math.floor(progress / size * 100) : 0}% · 已写入 {progress.toLocaleString('zh-HK')} 字节</span></div>}
      {cancelled && <p className="notice neutral" role="status">本次保存已取消，可重新开始。</p>}
      {complete && <p className="notice success" role="status">文件已保存到所选位置，接收长度与此版本一致。</p>}
      <div className="dialog-actions">
        {working ? <button className="button danger-outline" onClick={() => controller.current?.abort()}>取消下载</button>
          : <button className="button secondary" onClick={onClose} disabled={preparing}>关闭</button>}
        {!grant && <button className="button primary" onClick={() => void prepare()} disabled={preparing}>{preparing && <LoaderCircle size={16} className="spin" />}获取下载授权</button>}
        {grant && !complete && <button className="button primary" onClick={() => void save()} disabled={working || !picker}><Download size={16} />{working ? '正在分段保存' : '选择位置并保存'}</button>}
      </div>
    </div>
  </Dialog>;
}
