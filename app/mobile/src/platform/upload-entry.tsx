import Taro from '@tarojs/taro';
import type { Schemas } from './session';
import { Row } from '../ui';
export function UploadEntry({ folder, file }: { folder?: Schemas['Folder']; file?: Schemas['File'] }) {
  if (folder && !folder.can_upload || file && (!file.can_update || !file.current_version_id)) return null;
  const target = file ? '?file=' + file.id + '&base=' + file.current_version_id + '&folder=' + file.folder_id : folder ? '?folder=' + folder.id : '';
  return <Row title={file ? '上传新的内容版本' : folder ? '上传文件到这个目录' : '上传队列'} subtitle='普通附件／拍照票据；20 MiB以内，扫描后人工提交' onClick={() => void Taro.navigateTo({ url: '/pages/uploads/index' + target })} />;
}
