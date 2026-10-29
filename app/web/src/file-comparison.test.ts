import { describe, it } from 'vitest';
import { comparisonCases } from '../../packages/contracts/test/file-comparison-cases';

describe('complete fixed-version text comparisons', () => { for (const [name, run] of comparisonCases) it(name, run); });
