import { NativeModules } from 'react-native';
import { createContentSink } from './content-sink';
import { MobileError, type Scope } from './session';

export const reportFileAvailable = () => true;
export async function saveReportCSV(bytes: Uint8Array, kind: string, id: string, scope: Scope): Promise<void> {
  if (!/^(incurred|cash|forecast)$/.test(kind) || scope.closed || bytes.length > (16 << 20)) throw new MobileError('EXPORT_UNAVAILABLE', '导出读取已结束。');
  const sink = await createContentSink({ kind: 'document', extension: 'csv', mime: 'text/csv', limit: 16 << 20 }, id);
  const unbind = scope.own(() => { void sink.abort(); });
  let release: (() => void) | undefined;
  try {
    await sink.write(bytes);
    const artifact = await sink.finish();
    if (scope.closed || artifact.kind === 'text') throw new MobileError('REQUEST_CANCELLED', '导出读取已结束。');
    release = artifact.hold?.(); await NativeModules.CPReadCache.open(artifact.url);
  } finally { release?.(); unbind(); await sink.abort(); }
}
