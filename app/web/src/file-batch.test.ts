import { describe, it } from 'vitest';
import { fileBatchCases } from '../../packages/contracts/test/file-batch-cases';
describe('fixed human batch file plans', () => { for (const [name, run] of fileBatchCases) it(name, run); });
