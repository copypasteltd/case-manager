import { Text } from '@tarojs/components';
import { Page } from '../../ui';
export default function UploadView() {
  return <Page title='文件上传' home><Text className='cp-notice'>请使用 Web 或 Android 上传文件。本端可通过文件中心查阅资料及处理审批。</Text></Page>;
}
