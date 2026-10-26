import { client } from './taro';
import { MobileError, type Schemas } from './session';
import { UploadTransferController, type NativeUploadPort, type NativeUploadSource } from './upload-transfer';

function unsupported(): never { throw new MobileError('UPLOAD_CHANNEL_UNSUPPORTED', '请使用 Web 或 Android 上传文件。'); }
export const nativeUploads: NativeUploadPort & {
  select(replacement?: string): Promise<NativeUploadSource>;
  prepare(handle: string, target: Pick<Schemas['UploadCreate'], 'folder_id' | 'name' | 'file_id' | 'base_version_id'>): Promise<NativeUploadSource>;
  forget(handle: string): Promise<void>;
} = {
  scope: async () => unsupported(), list: async () => unsupported(), select: async () => unsupported(), prepare: async () => unsupported(),
  bind: async () => unsupported(), begin: async () => unsupported(), touch: async () => unsupported(), transfer: async () => unsupported(),
  verifySource: async () => unsupported(), finish: async () => unsupported(), pause: () => {}, forget: async () => unsupported(),
};
export const uploadTransfer = new UploadTransferController(client, nativeUploads);
export function observeNativeUploads() { return () => {}; }
