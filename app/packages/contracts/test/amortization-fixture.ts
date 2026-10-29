import type { AmortizationContent, AmortizationCost, AmortizationOverview, AmortizationVersion } from '../src/amortization';
export const amortizationID='11111111-1111-4111-8111-111111111111';
export const amortizationCostID='22222222-2222-4222-8222-222222222222';
export function amortizationCost():AmortizationCost {
  return {id:amortizationCostID,version:2,current_revision_id:amortizationID,content_hash:'a'.repeat(64),currency:'HKD',amount:'100.000000',allocations:[{project_id:amortizationID,amount:'60.000000'},{project_id:null,amount:'40.000000'}]};
}
export function amortizationContent():AmortizationContent {
  const dates=['2025-01-01','2025-02-01','2025-03-01','2025-04-01'],recognized=['2025-01-31','2025-02-28','2025-03-31'];
  return {cost_id:amortizationCostID,cost_version:2,cost_revision_id:amortizationID,cost_sha256:'a'.repeat(64),currency:'HKD',exponent:2,amount:'100.000000',method:'monthly_equal',rounding:'allocation_cumulative_half_up',service_from:dates[0],service_to:dates[3],
    periods:recognized.map((date,i)=>({sequence:i+1,service_from:dates[i],service_to:dates[i+1],recognized_on:date,amount:i===1?'33.340000':'33.330000',shares:[{project_id:amortizationID,amount:'20.000000'},{project_id:null,amount:i===1?'13.340000':'13.330000'}]}))};
}
export function amortizationVersion():AmortizationVersion{return {id:amortizationID,cost_id:amortizationCostID,version:1,state:'active',content_sha256:'b'.repeat(64),period_count:3,actor_id:amortizationID,reason:'Synthetic independently reviewed service period',created_at:'2025-01-01T00:00:00Z',content:amortizationContent()};}
export function amortizationOverview():AmortizationOverview{return {cost_id:amortizationCostID,version:1,can_manage:true,current:amortizationVersion()};}
