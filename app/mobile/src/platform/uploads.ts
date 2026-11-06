import Taro from '@tarojs/taro';
import { client } from './taro';
import { MobileError } from './session';
import { UploadTransferController } from './upload-transfer';
import { MiniUploadSources } from './mini-upload-source';

const fs = () => Taro.getFileSystemManager();
export const nativeUploads = new MiniUploadSources({
  key: () => client.operationKey(),
  async pick() {
    const wechat = process.env.TARO_ENV === 'weapp';
    const choice = await Taro.showActionSheet({ itemList: wechat ? ['拍照票据', '从相册选图片', '从微信会话选普通文件'] : ['拍照票据', '从相册选图片'] });
    if (wechat && choice.tapIndex === 2) {
      const result = await Taro.chooseMessageFile({ count: 1, type: 'file' }), file = result.tempFiles[0];
      if (!file) throw new MobileError('UPLOAD_CANCELLED', '已取消文件选择。');
      return { path: file.path, name: file.name, size: file.size, media: 'application/octet-stream' };
    }
    const result = await Taro.chooseImage({ count: 1, sourceType: [choice.tapIndex === 0 ? 'camera' : 'album'], sizeType: ['original'] });
    const file = result.tempFiles[0]; if (!file) throw new MobileError('UPLOAD_CANCELLED', '已取消图片选择。');
    const path = file.path, ext = /\.png$/i.test(path) ? 'png' : /\.webp$/i.test(path) ? 'webp' : 'jpg';
    return { path, name: '票据-' + Date.now() + '.' + ext, size: file.size, media: ext === 'jpg' ? 'image/jpeg' : 'image/' + ext };
  },
  size: path => new Promise((resolve, reject) => fs().getFileInfo({ filePath: path, success: value => resolve(value.size), fail: reject })),
  read: path => new Promise((resolve, reject) => fs().readFile({ filePath: path, success: value => typeof value.data === 'string' ? reject(new Error('文件读取未返回原始字节。')) : resolve(value.data), fail: reject })),
  put(url, headers, data) {
    const task = Taro.request({ url, method: 'PUT', header: headers, data, dataType: 'text', responseType: 'text', timeout: 60000 });
    return { abort: () => task.abort(), promise: task.then(value => ({ status: value.statusCode, etag: String(Object.entries(value.header).find(([key]) => key.toLowerCase() === 'etag')?.[1] ?? '') })) };
  },
});
client.subscribe(() => { if (!client.authenticated) nativeUploads.clear(); });
export const uploadTransfer = new UploadTransferController(client, nativeUploads);
export function observeNativeUploads() { return () => {}; }
