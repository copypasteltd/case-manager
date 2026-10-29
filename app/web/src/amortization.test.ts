import {it} from 'vitest';
import {strict as assert} from 'node:assert';
import {readFileSync} from 'node:fs';
import ts from 'typescript';
import {amortizationCases} from '../../packages/contracts/test/amortization-cases';
import {amortizationContent,amortizationCost} from '../../packages/contracts/test/amortization-fixture';
import type {checkedAmortizationContent} from '../../packages/contracts/src/amortization';
amortizationCases(it);

it('validates exact shares after legacy-target BigInt operator lowering',()=>{
  const source=readFileSync(new URL('../../packages/contracts/src/amortization.ts',import.meta.url),'utf8');
  const compiled=ts.transpileModule(source,{compilerOptions:{target:ts.ScriptTarget.ES2015,module:ts.ModuleKind.CommonJS}}).outputText;
  const exports={} as {checkedAmortizationContent:typeof checkedAmortizationContent};
  new Function('exports',compiled)(exports);
  const cost=amortizationCost();
  for(let exponent=0;exponent<=6;exponent+=1){
    const content={...amortizationContent(),exponent,service_to:'2025-02-01',periods:[{
      sequence:1,service_from:'2025-01-01',service_to:'2025-02-01',recognized_on:'2025-01-31',
      amount:cost.amount!,shares:cost.allocations,
    }]};
    assert.equal(exports.checkedAmortizationContent(content,cost),content);
  }
});
