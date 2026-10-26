import assert from 'node:assert/strict';

// Read the actual authorized source in bounded chunks, including every page.
export async function readReviewSource(request, origin, fixture, id, headers = {}) {
  const parts = []; let offset = 0, digest;
  for (let i = 0; i < 24; i++) {
    const response = await request.get(origin + '/v1/conversations/' + fixture.conversation_id + '/context-sources/agent_finance_report/' + id + '?context_security_version=1&offset=' + offset + '&max_chars=4000', { headers });
    assert.equal(response.status(), 200);
    const view = (await response.json()).data;
    assert.equal(view.offset, offset); assert.equal(view.version_id, null);
    if (digest) assert.equal(view.source_sha256, digest); digest = view.source_sha256;
    parts.push(view.text);
    if (view.next_offset === null) return { parts, digest, content: JSON.parse(parts.join('')) };
    assert.ok(view.next_offset > offset); offset = view.next_offset;
  }
  assert.fail('Source exceeded the bounded snapshot size');
}

export async function assertReviewFacts(request, origin, fixture, headers = {}) {
  const review = await readReviewSource(request, origin, fixture, fixture.finance_report_id, headers);
  const cost = await readReviewSource(request, origin, fixture, fixture.cost_report_id, headers);
  const payment = await readReviewSource(request, origin, fixture, fixture.payment_report_id, headers);
  assert.equal(review.content.view, 'subscription_review'); assert.equal(review.content.schema_version, 2);
  const value = review.content.report;
  assert.equal(value.subscription.id, fixture.subscription_id); assert.equal(value.coverage_complete, true);
  assert.equal(value.expected_scheduled_period_count, 1); assert.equal(value.periods.length, 1);
  assert.equal(value.periods[0].id, fixture.period_id); assert.equal(value.periods[0].amount, '100.000000');
  assert.ok(value.periods[0].anomalies.includes('invoice_missing')); assert.ok(value.periods[0].anomalies.includes('overcharge'));
  assert.deepEqual(value.costs.map(v => [v.id, v.original_amount, v.paid_amount, v.outstanding_amount]), [[fixture.cost_id, '100.120000', '40.000000', '60.120000']]);
  assert.deepEqual(value.payments.map(v => [v.id, v.original_amount]), [[fixture.payment_id, '40.000000']]);
  assert.deepEqual(value.allocations.map(v => [v.cost_id, v.payment_id, v.amount, v.shares[0].amount]), [[fixture.cost_id, fixture.payment_id, '40.000000', '40.000000']]);
  for (const [source, id, amount] of [[cost, fixture.cost_id, '100.120000'], [payment, fixture.payment_id, '40.000000']]) {
    assert.equal(source.content.view, 'finance_record'); assert.equal(source.content.report.record.id, id);
    assert.equal(source.content.report.record.amount, amount); assert.equal(source.content.report.record.attachments.length, 1);
    assert.equal(source.content.report.record_amount_basis, 'original_record_not_project_or_subscription_share');
    assert.doesNotMatch(source.parts.join(''), /cost_settlement|dependency_project_id|object_key|presigned_url/);
  }
  const ordinary = await request.get(origin + '/v1/costs/' + fixture.cost_id, { headers }); assert.equal(ordinary.status(), 200);
  const actual = (await ordinary.json()).data;
  assert.equal(actual.amount, cost.content.report.record.amount); assert.equal(actual.paid_amount, value.costs[0].paid_amount);
  assert.equal(actual.current_revision_id, cost.content.report.record.current_revision_id);
  return [review, cost, payment];
}

export async function orderReviewSources(request, origin, fixture, completed, reports, headers = {}) {
  const ids = [fixture.finance_report_id, fixture.cost_report_id, fixture.payment_report_id];
  const ordered = [];
  for (const source of completed.source_refs.filter(v => v.type === 'agent_finance_report')) {
    const index = ids.indexOf(source.id);
    const report = index < 0 ? await readReviewSource(request, origin, fixture, source.id, headers) : reports[index];
    assert.equal(source.sha256, report.digest);
    if (index < 0) {
      assert.equal(report.content.view, 'subscription_review'); assert.equal(report.content.report.coverage_complete, false);
      assert.equal(report.content.report.missing_scheduled_periods.length, 1); assert.equal(report.content.report.costs.length, 0);
    }
    ordered.push(report);
  }
  // The controller also preserves the genuine February gap observation.
  assert.equal(ordered.length, 4);
  return ordered;
}
