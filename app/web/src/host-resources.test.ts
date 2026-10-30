import { describe, it } from 'vitest';
import { hostResourceCases } from '../../packages/contracts/test/host-resource-cases';
describe('Host resource business evidence', () => { for (const [name, run] of hostResourceCases) it(name, run); });
