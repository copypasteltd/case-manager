import Taro from '@tarojs/taro';
import type { Schemas } from './session';
import { checkedOfficeArtifact } from '@copypaste/contracts/office-author';
import { nativeUploads } from './uploads.rn';
import { client } from './taro';
export const officeWritable = true;
export async function queueOfficeArtifact(value: Schemas['OfficeArtifact']): Promise<void> {
  const artifact = checkedOfficeArtifact(value);
  const epoch = client.epoch;
  const me = await client.request<Schemas['Me']>('/me');
  if (epoch !== client.epoch || !client.authenticated) throw new Error('登录状态已变化，请重新打开文档。');
  await nativeUploads.scope(me.user.id);
  const source = await nativeUploads.generated(artifact);
  await nativeUploads.prepare(source.handle, { folder_id: artifact.upload.folder_id, name: artifact.name, ...(artifact.upload.file_id ? { file_id: artifact.upload.file_id } : {}), ...(artifact.upload.base_version_id ? { base_version_id: artifact.upload.base_version_id } : {}) });
  await Taro.navigateTo({ url: '/pages/uploads/index' });
}
