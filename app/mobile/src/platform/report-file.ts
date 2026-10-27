import Taro from '@tarojs/taro';
import { MobileError, type Scope } from './session';

export const reportFileAvailable = () => process.env.TARO_ENV === 'h5' || process.env.TARO_ENV === 'weapp' && Taro.canIUse('shareFileMessage');
export async function saveReportCSV(bytes: Uint8Array, kind: string, id: string, scope: Scope): Promise<void> {
  if (!/^(incurred|cash|forecast)$/.test(kind) || !/^[a-f0-9]{8}-(?:[a-f0-9]{4}-){3}[a-f0-9]{12}$/.test(id) || bytes.length > (16 << 20) || scope.closed) throw new MobileError('EXPORT_UNAVAILABLE', '导出读取已结束，请重新核对任务。');
  const filename = 'finance-' + kind + '-' + id + '.csv';
  if (process.env.TARO_ENV === 'h5') {
    const url = URL.createObjectURL(new Blob([bytes.slice().buffer as ArrayBuffer], { type: 'text/csv;charset=utf-8' }));
    try { const anchor = document.createElement('a'); anchor.href = url; anchor.download = filename; anchor.click(); } finally { setTimeout(() => URL.revokeObjectURL(url), 1000); }
    return;
  }
  if (!reportFileAvailable()) throw new MobileError('EXPORT_PLATFORM_UNAVAILABLE', '本端支持报表查询与排队；CSV 文件请在 Web 下载。');
  const fs = Taro.getFileSystemManager(), path = Taro.env.USER_DATA_PATH + '/' + filename;
  const remove = () => new Promise<void>(resolve => fs.unlink({ filePath: path, complete: () => resolve() }));
  const unbind = scope.own(() => { void remove(); });
  try {
    await new Promise<void>((resolve, reject) => fs.writeFile({ filePath: path, data: bytes.slice().buffer as ArrayBuffer, success: () => resolve(), fail: reject }));
    if (scope.closed) throw new MobileError('REQUEST_CANCELLED', '导出读取已结束。');
    await Taro.shareFileMessage({ filePath: path, fileName: filename });
  } finally { unbind(); await remove(); }
}
