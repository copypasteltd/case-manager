-- Current closeout uses original refund principal units. Existing journal,
-- archive and completion snapshot bytes remain immutable.
CREATE FUNCTION payment_refund_unattributed_principal(oid uuid,pid uuid) RETURNS numeric LANGUAGE sql STABLE AS $$
 SELECT CASE WHEN p.record_kind='payment' AND p.state='recorded' AND r.direction='refund'
 THEN greatest(0,COALESCE(r.refund_original_amount,r.amount)
  +COALESCE((SELECT sum(a.effect*a.amount) FROM payment_allocations a WHERE a.org_id=oid AND a.payment_id=p.id),0)
  -COALESCE((SELECT sum(contract_finance_remaining(oid,e.id)) FROM contract_finance_events e WHERE e.org_id=oid AND e.payment_id=p.id AND e.kind='refund'),0))
 ELSE 0 END FROM payment_records p JOIN payment_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id WHERE p.org_id=oid AND p.id=pid
$$;
CREATE FUNCTION payment_refund_principal_currency(oid uuid,pid uuid) RETURNS text LANGUAGE sql STABLE AS $$
 SELECT COALESCE(original.currency,r.currency) FROM payment_records p JOIN payment_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id
 LEFT JOIN payment_records root ON root.org_id=p.org_id AND root.id=r.refund_of_id
 LEFT JOIN payment_revisions original ON original.org_id=root.org_id AND original.id=root.current_revision_id WHERE p.org_id=oid AND p.id=pid
$$;

CREATE OR REPLACE FUNCTION purchase_completion_snapshot(oid uuid,pid uuid) RETURNS jsonb LANGUAGE sql STABLE AS $$
    SELECT jsonb_build_object('revision_id',r.id,'content_hash',r.content_hash,'currency',r.currency,
        'execution_plan_revision_id',plan.revision_id,'execution_amount',plan.amount::text,'lines',purchase_fulfillment_lines(oid,pid),
        'orders',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',o.id,'purchase_version',o.purchase_version,'execution_plan_revision_id',o.execution_plan_revision_id,
            'external_ref',o.external_ref,'supplier_id',o.supplier_id,'supplier_revision_id',o.supplier_revision_id,'ordered_on',o.ordered_on,'amount',o.amount::text,
            'cancelled',EXISTS(SELECT 1 FROM purchase_execution_events e WHERE e.org_id=oid AND e.order_id=o.id AND e.kind='order_cancelled'),
            'lines',(SELECT jsonb_agg(jsonb_build_object('id',l.id,'purchase_line_id',l.purchase_line_id,'description',l.description,'unit',l.unit,'quantity',l.quantity::text,
                'unit_price',l.unit_price::text,'tax_amount',l.tax_amount::text,'discount_amount',l.discount_amount::text,'amount',l.amount::text,
                'net_accepted_quantity',purchase_net_accepted(oid,l.id)::numeric(20,6)::text) ORDER BY l.position) FROM purchase_order_lines l WHERE l.org_id=oid AND l.order_id=o.id)) ORDER BY o.purchase_version)
            FROM purchase_orders o WHERE o.org_id=oid AND o.purchase_id=pid),'[]'::jsonb),
        'receipts',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',q.id,'order_id',q.order_id,'purchase_version',q.purchase_version,'received_on',q.received_on,
            'lines',(SELECT jsonb_agg(jsonb_build_object('id',l.id,'order_line_id',l.order_line_id,'quantity',l.quantity::text,'result',l.result,'reverses_line_id',l.reverses_line_id,'reason',l.reason) ORDER BY l.position)
                FROM purchase_receipt_lines l WHERE l.org_id=oid AND l.receipt_id=q.id)) ORDER BY q.purchase_version) FROM purchase_receipts q WHERE q.org_id=oid AND q.purchase_id=pid),'[]'::jsonb),
        'variances',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',v.id,'version',v.version,'revision_id',v.current_revision_id,'content_hash',vr.content_hash,
            'title',vr.payload->>'title','approval_state',v.approval_state,'lifecycle',v.lifecycle,'basis_purchase_version',vr.basis->'purchase_version') ORDER BY v.id)
            FROM purchase_variances v JOIN purchase_variance_revisions vr ON vr.org_id=v.org_id AND vr.id=v.current_revision_id WHERE v.org_id=oid AND v.purchase_id=pid),'[]'::jsonb),
        'costs',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',d.id,'version',d.version,'revision_id',cr.id,'content_hash',cr.content_hash,'title',cr.title,'entry_kind',d.entry_kind,
            'state',d.state,'currency',cr.currency,'amount',cr.amount::text,'paid_amount',COALESCE((SELECT sum(a.effect*a.cost_amount) FROM payment_allocations a WHERE a.org_id=oid AND a.cost_id=d.id),0)::text)
            ORDER BY d.id) FROM actual_cost_entries d JOIN cost_revisions cr ON cr.org_id=d.org_id AND cr.id=d.current_revision_id WHERE d.org_id=oid AND d.id IN(SELECT id FROM purchase_completion_cost_ids(oid,pid))),'[]'::jsonb),
        'payments',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',p.id,'version',p.version,'revision_id',pr.id,'content_hash',pr.content_hash,'state',p.state,'record_kind',p.record_kind,
            'direction',pr.direction,'amount',pr.amount::text,'currency',pr.currency,'refund_of_id',pr.refund_of_id,
            'unattributed_refund_amount',payment_refund_unattributed_principal(oid,p.id)::text,
             'unattributed_refund_currency',payment_refund_principal_currency(oid,p.id),
             'allocations',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',a.id,'cost_id',a.cost_id,'cost_revision_id',a.cost_revision_id,'kind',a.kind,'effect',a.effect,'amount',a.amount::text) ORDER BY a.id)
                FROM payment_allocations a WHERE a.org_id=oid AND a.payment_id=p.id),'[]'::jsonb)) ORDER BY p.id)
            FROM payment_records p JOIN payment_revisions pr ON pr.org_id=p.org_id AND pr.id=p.current_revision_id WHERE p.org_id=oid AND p.id IN(SELECT id FROM purchase_completion_cash_ids(oid,pid))),'[]'::jsonb))
    FROM spend_documents d JOIN spend_revisions r ON r.org_id=d.org_id AND r.id=d.approved_revision_id
    CROSS JOIN LATERAL purchase_effective_plan(oid,pid) plan WHERE d.org_id=oid AND d.id=pid AND d.kind='purchase'
$$;

CREATE OR REPLACE FUNCTION project_closeout_items(oid uuid,pid uuid) RETURNS TABLE(kind text,id uuid,version bigint,title text,state text,blocking boolean,facts jsonb) LANGUAGE sql STABLE AS $$
 SELECT 'tasks',t.id,t.version,t.title,t.state,t.state NOT IN('done','cancelled'),to_jsonb(t) FROM tasks t WHERE t.org_id=oid AND t.project_id=pid
 UNION ALL SELECT 'milestones',m.id,m.version,m.title,m.state,m.state NOT IN('achieved','cancelled'),to_jsonb(m) FROM milestones m WHERE m.org_id=oid AND m.project_id=pid
 UNION ALL SELECT 'deliverables',d.id,d.version,d.name,d.state,d.state<>'accepted' OR d.protocol_version<>1,
  jsonb_build_object('head',to_jsonb(d),'revision',to_jsonb(r),'decision',to_jsonb(a)) FROM deliverables d
  LEFT JOIN deliverable_revisions r ON r.org_id=d.org_id AND r.id=d.current_revision_id LEFT JOIN acceptance_decisions a ON a.org_id=r.org_id AND a.revision_id=r.id WHERE d.org_id=oid AND d.project_id=pid
 UNION ALL SELECT 'risks',r.id,r.version,r.title,r.state,r.state NOT IN('closed','realized'),to_jsonb(r) FROM risks r WHERE r.org_id=oid AND r.project_id=pid
 UNION ALL SELECT 'issues',i.id,i.version,i.title,i.state,i.state<>'closed',to_jsonb(i) FROM issues i WHERE i.org_id=oid AND i.project_id=pid
 UNION ALL SELECT 'changes',c.id,c.version,c.title,c.state,c.state NOT IN('implemented','rejected','withdrawn') OR c.protocol_version<>1,
  jsonb_build_object('head',to_jsonb(c),'revision',to_jsonb(r)) FROM change_requests c LEFT JOIN project_change_revisions r ON r.org_id=c.org_id AND r.id=c.current_revision_id WHERE c.org_id=oid AND c.project_id=pid
 UNION ALL SELECT 'budgets',b.id,b.revision,b.currency,b.state,false,jsonb_build_object('head',to_jsonb(b),'lines',COALESCE((SELECT jsonb_agg(to_jsonb(l) ORDER BY l.id) FROM budget_lines l WHERE l.org_id=oid AND l.budget_id=b.id),'[]'::jsonb)) FROM project_budgets b WHERE b.org_id=oid AND b.project_id=pid
 UNION ALL SELECT 'contracts',c.id,c.version,r.title,c.lifecycle,
  c.lifecycle NOT IN('completed','terminated') OR c.approval_state='in_review' OR c.active_revision_id IS NULL OR COALESCE(contract_closure_has_unsettled(contract_closure_snapshot(oid,c.id)),true),
  jsonb_build_object('head',to_jsonb(c),'revision',to_jsonb(r),'financial',contract_closure_snapshot(oid,c.id),'closure',(SELECT to_jsonb(h) FROM contract_closures h WHERE h.org_id=oid AND h.contract_id=c.id))
  FROM contracts c JOIN contract_revisions r ON r.org_id=c.org_id AND r.id=c.current_revision_id WHERE c.org_id=oid AND c.project_id=pid AND c.deleted_at IS NULL
 UNION ALL SELECT 'purchases',d.id,d.version,r.title,d.lifecycle,
  d.approval_state NOT IN('approved','rejected','withdrawn') OR d.approval_state='approved' AND (d.lifecycle NOT IN('completed','cancelled') OR COALESCE(cardinality(CASE WHEN d.lifecycle='cancelled' THEN array_remove(purchase_completion_blockers(purchase_completion_snapshot(oid,d.id)),'no_accepted_delivery') ELSE purchase_completion_blockers(purchase_completion_snapshot(oid,d.id)) END)>0,true)),
  jsonb_build_object('head',to_jsonb(d),'revision',to_jsonb(r),'fulfillment',purchase_completion_snapshot(oid,d.id),'completion',(SELECT to_jsonb(h) FROM purchase_completions h WHERE h.org_id=oid AND h.purchase_id=d.id))
  FROM spend_documents d JOIN spend_revisions r ON r.org_id=d.org_id AND r.id=d.current_revision_id WHERE d.org_id=oid AND d.project_id=pid AND d.kind='purchase' AND d.deleted_at IS NULL
 UNION ALL SELECT 'claims',d.id,d.version,r.title,d.approval_state,
  d.approval_state NOT IN('approved','rejected','withdrawn') OR d.approval_state='approved' AND (
   COALESCE((SELECT sum((l->>'declared_amount')::numeric) FROM jsonb_array_elements(ar.payload->'lines')l WHERE l->>'funding'='employee_advance'),0)
    <>COALESCE((SELECT sum(a.effect*a.cost_amount) FROM payment_allocations a WHERE a.org_id=oid AND a.claim_id=d.id AND a.claim_revision_id=d.approved_revision_id),0)
   OR COALESCE((SELECT sum(cr.amount) FROM cost_links l JOIN actual_cost_entries c ON c.org_id=l.org_id AND c.id=l.cost_id JOIN cost_revisions cr ON cr.org_id=c.org_id AND cr.id=c.current_revision_id
    WHERE l.org_id=oid AND l.source_type='claim' AND l.source_id=d.id AND l.revision_id=d.approved_revision_id AND c.entry_kind='expense' AND c.state='confirmed' AND cr.currency=ar.currency),0) IS DISTINCT FROM ar.amount),
  jsonb_build_object('head',to_jsonb(d),'revision',to_jsonb(r),'approved_revision',to_jsonb(ar),'allocations',COALESCE((SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM payment_allocations a WHERE a.org_id=oid AND a.claim_id=d.id),'[]'::jsonb))
  FROM spend_documents d JOIN spend_revisions r ON r.org_id=d.org_id AND r.id=d.current_revision_id LEFT JOIN spend_revisions ar ON ar.org_id=d.org_id AND ar.id=d.approved_revision_id WHERE d.org_id=oid AND d.project_id=pid AND d.kind='claim' AND d.deleted_at IS NULL
 UNION ALL SELECT 'costs',c.id,c.version,r.title,c.state,c.state='draft' OR c.entry_kind='expense' AND
  COALESCE((SELECT sum(a.effect*a.cost_amount) FROM payment_allocations a WHERE a.org_id=oid AND a.cost_id=c.id),0)<>CASE WHEN c.state='confirmed' THEN r.amount ELSE 0 END,
  jsonb_build_object('head',to_jsonb(c),'revision',to_jsonb(r),'paid_amount',COALESCE((SELECT sum(a.effect*a.cost_amount) FROM payment_allocations a WHERE a.org_id=oid AND a.cost_id=c.id),0)::text,
   'allocations',COALESCE((SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM payment_allocations a WHERE a.org_id=oid AND a.cost_id=c.id),'[]'::jsonb),
   'links',COALESCE((SELECT jsonb_agg(to_jsonb(l) ORDER BY l.id) FROM cost_links l WHERE l.org_id=oid AND l.cost_id=c.id),'[]'::jsonb))
  FROM actual_cost_entries c JOIN cost_revisions r ON r.org_id=c.org_id AND r.id=c.current_revision_id WHERE c.org_id=oid AND c.id IN(SELECT id FROM project_closeout_cost_ids(oid,pid))
 UNION ALL SELECT 'payments',p.id,p.version,r.title,p.state,p.state='draft' OR payment_refund_unattributed_principal(oid,p.id)>0,
  jsonb_build_object('head',to_jsonb(p),'revision',to_jsonb(r),'refund_principal_currency',payment_refund_principal_currency(oid,p.id),'unattributed_refund_principal',payment_refund_unattributed_principal(oid,p.id)::text,'allocations',COALESCE((SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM payment_allocations a WHERE a.org_id=oid AND a.payment_id=p.id),'[]'::jsonb),
   'contract_events',COALESCE((SELECT jsonb_agg(to_jsonb(e) ORDER BY e.id) FROM contract_finance_events e WHERE e.org_id=oid AND e.payment_id=p.id),'[]'::jsonb))
  FROM payment_records p JOIN payment_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id WHERE p.org_id=oid AND p.id IN(SELECT id FROM project_closeout_cash_ids(oid,pid))
 UNION ALL SELECT 'subscriptions',s.id,s.version,r.product,s.state,s.state NOT IN('cancelled','archived') OR s.cancel_effective_on>(clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date OR
  r.payload->>'vendor_state' IS DISTINCT FROM 'cancelled' OR r.payload->>'vendor_checked_at' IS NULL OR COALESCE(jsonb_array_length(r.payload->'vendor_evidence_version_ids'),0)=0 OR
  EXISTS(SELECT 1 FROM subscription_periods p WHERE p.org_id=oid AND p.subscription_id=s.id AND (p.state='forecast' OR p.state='confirmed' AND p.review_fingerprint IS DISTINCT FROM subscription_actual_fingerprint(oid,p.id))),
  jsonb_build_object('head',to_jsonb(s),'revision',to_jsonb(r),'periods',COALESCE((SELECT jsonb_agg(jsonb_build_object('head',to_jsonb(p),'revision',to_jsonb(pr),'actual_fingerprint',subscription_actual_fingerprint(oid,p.id)) ORDER BY p.id)
   FROM subscription_periods p JOIN subscription_period_revisions pr ON pr.org_id=p.org_id AND pr.id=p.current_revision_id WHERE p.org_id=oid AND p.subscription_id=s.id),'[]'::jsonb))
  FROM subscriptions s JOIN subscription_revisions r ON r.org_id=s.org_id AND r.id=s.current_revision_id WHERE s.org_id=oid AND s.project_id=pid
$$;
