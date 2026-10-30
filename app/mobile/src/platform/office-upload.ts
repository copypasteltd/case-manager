import type { Schemas } from './session';
export const officeWritable = false;
export async function queueOfficeArtifact(_artifact: Schemas['OfficeArtifact']): Promise<void> { throw new Error('请在 Web 或 Android 创建和修改文档。'); }
