import { createHash } from 'node:crypto';
import { describe, expect, it } from 'vitest';
import { checkedFinancialReport, checkedReportExport, currentReportQuery, financialReportPath, formatReportAmount, monthWindow, readReportExportBytes, REPORT_CHUNK_BYTES, reportQueryKey, validateReportQuery, type FinancialReport, type ReportExport, type ReportExportChunk } from '@copypaste/contracts/finance-reports';

const id = '11111111-1111-4111-8111-111111111111', job = '22222222-2222-4222-8222-222222222222';
const query = { ...currentReportQuery(), from: '2025-01-01', to: '2025-02-01' };
const digest = (bytes: Uint8Array) => createHash('sha256').update(bytes).digest('hex');
const csv = Buffer.alloc(REPORT_CHUNK_BYTES + 37, 65);
const record: ReportExport = { id, job_id: job, filters: query, state: 'succeeded', attempt: 1, error_code: null, created_at: '2026-10-27T01:00:00Z', expires_at: '2026-10-28T01:00:00Z', as_of: '2026-10-27T01:00:01Z', row_count: 27, csv_bytes: csv.length, csv_sha256: digest(csv), can_cancel: false, can_download: true };
function chunk(offset: number, length: number): ReportExportChunk { const data = csv.subarray(offset, offset + length); return { export_id: id, filename: 'finance-incurred-' + id + '.csv', total_size_bytes: String(csv.length), sha256: digest(csv), offset: String(offset), length: String(data.length), chunk_sha256: digest(data), data_base64: data.toString('base64') }; }
function report(): FinancialReport { return { filters: query, scope: 'authorized_records_only', summary: { kind: 'incurred', from: query.from, to: query.to, timezone: 'Asia/Hong_Kong', as_of: record.as_of!, base_currency: null, fx_coverage: 'unconfigured', totals: [{ currency: 'HKD', expense_amount: '100.000000', reversal_amount: '0.000000', net_amount: '100.000000', entry_count: 1 }], unconfirmed_count: 0 }, items: [], next_cursor: null }; }

describe('financial report boundaries', () => {
  it('validates real calendar windows and every signed filter', () => {
    expect(monthWindow('2024-02')).toEqual({ from: '2024-02-01', to: '2024-03-01' });
    expect(monthWindow('2025-12').to).toBe('2026-01-01');
    for (const changed of [{ from: '2025-02-30' }, { to: query.from }, { to: '2027-01-01' }, { currency: 'hkd' }, { owner_id: '//elsewhere' }, { project_id: id, company_only: true }]) expect(() => validateReportQuery({ ...query, ...changed })).toThrow();
    for (const changed of [{ category: 'cloud' }, { q: 'bill' }, { owner_id: id }, { supplier_id: id }, { project_id: id }, { company_only: true }, { currency: 'HKD' }, { kind: 'cash' as const }]) expect(reportQueryKey({ ...query, ...changed })).not.toBe(reportQueryKey(query));
    expect(financialReportPath({ ...query, q: 'name & q', supplier_id: id })).toContain('q=name+%26+q');
  });
  it('keeps large and negative decimal amounts exact', () => {
    expect(formatReportAmount('99999999999999.999999')).toBe('99999999999999.999999');
    expect(formatReportAmount('-60.000000')).toBe('-60');
    expect(formatReportAmount('0.000001')).toBe('0.000001');
  });
  it('rejects a report from another filter or mixed/guessed currency basis', () => {
    expect(checkedFinancialReport(report(), query).summary.kind).toBe('incurred');
    expect(() => checkedFinancialReport(report(), { ...query, category: 'other' })).toThrow();
    const changed = report(); if ('net_amount' in changed.summary.totals[0]) changed.summary.totals[0].net_amount = '1e2';
    expect(() => checkedFinancialReport(changed, query)).toThrow();
  });
  it('never treats a queued or inconsistent job as a generated file', () => {
    expect(checkedReportExport(record).can_download).toBe(true);
    expect(() => checkedReportExport({ ...record, state: 'queued' })).toThrow();
    expect(() => checkedReportExport({ ...record, csv_bytes: 17 << 20 })).toThrow();
    expect(() => checkedReportExport(record, undefined, job)).toThrow();
    expect(() => checkedReportExport(record, { ...query, currency: 'USD' })).toThrow();
  });
  it('assembles actual bounded segments and verifies the complete file', async () => {
    const ranges: number[][] = [];
    const result = await readReportExportBytes(record, async (offset, length) => { ranges.push([offset, length]); return chunk(offset, length); }, digest, () => {});
    expect(ranges).toEqual([[0, REPORT_CHUNK_BYTES], [REPORT_CHUNK_BYTES, 37]]);
    expect(digest(result)).toBe(digest(csv)); result.fill(0);
  });
  it('rejects foreign, reordered, oversized or corrupted response segments', async () => {
    for (const edit of [{ export_id: job }, { offset: '1' }, { length: '999999' }, { filename: '../other.csv' }, { sha256: 'f'.repeat(64) }, { data_base64: 'AAAA' }, { data_base64: 'A===' }]) await expect(readReportExportBytes(record, async (offset, length) => ({ ...chunk(offset, length), ...edit }), digest, () => {})).rejects.toThrow();
  });
  it('clears decoded bytes when a hash operation fails', async () => {
    let retained: Uint8Array | undefined;
    await expect(readReportExportBytes(record, async (offset, length) => chunk(offset, length), input => { retained = input; throw new Error('hash failed'); }, () => {})).rejects.toThrow('hash failed');
    expect(retained?.every(value => value === 0)).toBe(true);
  });
  it('cancels during digest verification before exposing a completed file', async () => {
    let active = true;
    await expect(readReportExportBytes(record, async (offset, length) => chunk(offset, length), async input => { const hash = digest(input); if (input.length === csv.length) active = false; return hash; }, () => { if (!active) throw new Error('page closed'); })).rejects.toThrow('page closed');
  });
});
