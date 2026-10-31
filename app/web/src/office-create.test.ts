import { describe, it } from 'vitest';
import { officeCreateCases } from '../../packages/contracts/test/office-create-cases';
describe('exact Office file creation and real artifact receipts', () => { for (const [name, check] of officeCreateCases) it(name, check); });
