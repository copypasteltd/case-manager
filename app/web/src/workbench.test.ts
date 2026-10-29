import { describe, it } from 'vitest';
import { workbenchCases } from '../../packages/contracts/test/workbench-cases';
describe('current-authority business workbench', () => { for (const [name, run] of workbenchCases) it(name, run); });
