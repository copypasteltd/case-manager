-- Exact, human-attested project archive; never changes contractual or financial facts.
CREATE FUNCTION project_closeout_cost_ids(oid uuid,pid uuid) RETURNS TABLE(id uuid) LANGUAGE sql STABLE AS $$
 SELECT d.id FROM actual_cost_entries d WHERE d.org_id=oid AND (d.project_id=pid
 OR EXISTS(SELECT 1 FROM cost_allocations a WHERE a.org_id=oid AND a.revision_id=d.current_revision_id AND a.project_id=pid)
 OR EXISTS(SELECT 1 FROM cost_links l WHERE l.org_id=oid AND l.cost_id=d.id AND (
  l.source_type='contract' AND EXISTS(SELECT 1 FROM contracts c WHERE c.org_id=oid AND c.id=l.source_id AND c.project_id=pid)
  OR l.source_type IN('claim','purchase') AND EXISTS(SELECT 1 FROM spend_documents s WHERE s.org_id=oid AND s.id=l.source_id AND s.project_id=pid)
  OR l.source_type='subscription_period' AND EXISTS(SELECT 1 FROM subscription_periods p JOIN subscriptions s ON s.org_id=p.org_id AND s.id=p.subscription_id WHERE p.org_id=oid AND p.id=l.source_id AND s.project_id=pid))))
$$;
CREATE FUNCTION project_closeout_cash_ids(oid uuid,pid uuid) RETURNS TABLE(id uuid) LANGUAGE sql STABLE AS $$
 WITH RECURSIVE base(id) AS(SELECT p.id FROM payment_records p WHERE p.org_id=oid AND p.project_id=pid
  UNION SELECT a.payment_id FROM payment_allocations a WHERE a.org_id=oid AND a.cost_id IN(SELECT id FROM project_closeout_cost_ids(oid,pid))
  UNION SELECT e.payment_id FROM contract_finance_events e JOIN contracts c ON c.org_id=e.org_id AND c.id=e.contract_id WHERE e.org_id=oid AND c.project_id=pid AND e.payment_id IS NOT NULL),
 roots(id) AS(SELECT id FROM base UNION SELECT p.reverses_id FROM payment_records p WHERE p.org_id=oid AND p.id IN(SELECT id FROM base)
  UNION SELECT r.refund_of_id FROM payment_records p JOIN payment_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id WHERE p.org_id=oid AND p.id IN(SELECT id FROM base)),
 family(id) AS(SELECT id FROM roots WHERE id IS NOT NULL UNION SELECT p.id FROM payment_records p JOIN payment_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id
  JOIN family f ON p.reverses_id=f.id OR r.refund_of_id=f.id WHERE p.org_id=oid)
 SELECT id FROM family
$$;
-- Every row is a complete concurrency fact, not a projected balance or an approval amount.
CREATE FUNCTION project_closeout_items(oid uuid,pid uuid) RETURNS TABLE(kind text,id uuid,version bigint,title text,state text,blocking boolean,facts jsonb) LANGUAGE sql STABLE AS $$
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
    <>COALESCE((SELECT sum(a.effect*a.amount) FROM payment_allocations a WHERE a.org_id=oid AND a.claim_id=d.id AND a.claim_revision_id=d.approved_revision_id),0)
   OR COALESCE((SELECT sum(cr.amount) FROM cost_links l JOIN actual_cost_entries c ON c.org_id=l.org_id AND c.id=l.cost_id JOIN cost_revisions cr ON cr.org_id=c.org_id AND cr.id=c.current_revision_id
    WHERE l.org_id=oid AND l.source_type='claim' AND l.source_id=d.id AND l.revision_id=d.approved_revision_id AND c.entry_kind='expense' AND c.state='confirmed' AND cr.currency=ar.currency),0) IS DISTINCT FROM ar.amount),
  jsonb_build_object('head',to_jsonb(d),'revision',to_jsonb(r),'approved_revision',to_jsonb(ar),'allocations',COALESCE((SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM payment_allocations a WHERE a.org_id=oid AND a.claim_id=d.id),'[]'::jsonb))
  FROM spend_documents d JOIN spend_revisions r ON r.org_id=d.org_id AND r.id=d.current_revision_id LEFT JOIN spend_revisions ar ON ar.org_id=d.org_id AND ar.id=d.approved_revision_id WHERE d.org_id=oid AND d.project_id=pid AND d.kind='claim' AND d.deleted_at IS NULL
 UNION ALL SELECT 'costs',c.id,c.version,r.title,c.state,c.state='draft' OR c.entry_kind='expense' AND
  COALESCE((SELECT sum(a.effect*a.amount) FROM payment_allocations a WHERE a.org_id=oid AND a.cost_id=c.id),0)<>CASE WHEN c.state='confirmed' THEN r.amount ELSE 0 END,
  jsonb_build_object('head',to_jsonb(c),'revision',to_jsonb(r),'paid_amount',COALESCE((SELECT sum(a.effect*a.amount) FROM payment_allocations a WHERE a.org_id=oid AND a.cost_id=c.id),0)::text,
   'allocations',COALESCE((SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM payment_allocations a WHERE a.org_id=oid AND a.cost_id=c.id),'[]'::jsonb),
   'links',COALESCE((SELECT jsonb_agg(to_jsonb(l) ORDER BY l.id) FROM cost_links l WHERE l.org_id=oid AND l.cost_id=c.id),'[]'::jsonb))
  FROM actual_cost_entries c JOIN cost_revisions r ON r.org_id=c.org_id AND r.id=c.current_revision_id WHERE c.org_id=oid AND c.id IN(SELECT id FROM project_closeout_cost_ids(oid,pid))
 UNION ALL SELECT 'payments',p.id,p.version,r.title,p.state,p.state='draft' OR p.state='recorded' AND p.record_kind='payment' AND r.direction='refund' AND
  r.amount+COALESCE((SELECT sum(a.effect*a.amount) FROM payment_allocations a WHERE a.org_id=oid AND a.payment_id=p.id),0)>
  COALESCE((SELECT sum(contract_finance_remaining(oid,e.id)) FROM contract_finance_events e WHERE e.org_id=oid AND e.payment_id=p.id AND e.kind='refund'),0),
  jsonb_build_object('head',to_jsonb(p),'revision',to_jsonb(r),'allocations',COALESCE((SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM payment_allocations a WHERE a.org_id=oid AND a.payment_id=p.id),'[]'::jsonb),
   'contract_events',COALESCE((SELECT jsonb_agg(to_jsonb(e) ORDER BY e.id) FROM contract_finance_events e WHERE e.org_id=oid AND e.payment_id=p.id),'[]'::jsonb))
  FROM payment_records p JOIN payment_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id WHERE p.org_id=oid AND p.id IN(SELECT id FROM project_closeout_cash_ids(oid,pid))
 UNION ALL SELECT 'subscriptions',s.id,s.version,r.product,s.state,s.state NOT IN('cancelled','archived') OR s.cancel_effective_on>(clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date OR
  r.payload->>'vendor_state' IS DISTINCT FROM 'cancelled' OR r.payload->>'vendor_checked_at' IS NULL OR COALESCE(jsonb_array_length(r.payload->'vendor_evidence_version_ids'),0)=0 OR
  EXISTS(SELECT 1 FROM subscription_periods p WHERE p.org_id=oid AND p.subscription_id=s.id AND (p.state='forecast' OR p.state='confirmed' AND p.review_fingerprint IS DISTINCT FROM subscription_actual_fingerprint(oid,p.id))),
  jsonb_build_object('head',to_jsonb(s),'revision',to_jsonb(r),'periods',COALESCE((SELECT jsonb_agg(jsonb_build_object('head',to_jsonb(p),'revision',to_jsonb(pr),'actual_fingerprint',subscription_actual_fingerprint(oid,p.id)) ORDER BY p.id)
   FROM subscription_periods p JOIN subscription_period_revisions pr ON pr.org_id=p.org_id AND pr.id=p.current_revision_id WHERE p.org_id=oid AND p.subscription_id=s.id),'[]'::jsonb))
  FROM subscriptions s JOIN subscription_revisions r ON r.org_id=s.org_id AND r.id=s.current_revision_id WHERE s.org_id=oid AND s.project_id=pid
$$;
CREATE FUNCTION project_closeout_snapshot(oid uuid,pid uuid) RETURNS jsonb LANGUAGE sql STABLE AS $$
 SELECT jsonb_build_object('schema_version',1,'project',to_jsonb(p),'items',COALESCE((SELECT jsonb_agg(to_jsonb(i) ORDER BY i.kind,i.id) FROM project_closeout_items(oid,pid)i),'[]'::jsonb)) FROM projects p WHERE p.org_id=oid AND p.id=pid
$$;
CREATE FUNCTION project_closeout_readable_groups(oid uuid,uid uuid,pid uuid,ch text,ai boolean) RETURNS TABLE(kind text,readable boolean) LANGUAGE sql STABLE AS $$
 WITH kinds(kind,action,rtype) AS(VALUES('tasks','task.read','task'),('milestones','milestone.read','milestone'),('deliverables','deliverable.read','deliverable'),('risks','risk.read','risk'),('issues','issue.read','issue'),('changes','change.read','change'),('budgets','budget.read','project'),('contracts','contract.read','contract'),('purchases','purchase.read','purchase'),('claims','claim.read','claim'),('costs','cost.read','cost'),('payments','payment.read','payment'),('subscriptions','subscription.read','subscription')),items AS MATERIALIZED(SELECT * FROM project_closeout_items(oid,pid))
 SELECT k.kind,can_access(oid,uid,k.action,k.rtype,pid,pid,p.creator_id,p.classification,ch,ai) AND
 NOT EXISTS(SELECT 1 FROM items i WHERE i.kind=k.kind AND NOT CASE k.kind
  WHEN 'budgets' THEN can_access(oid,uid,'budget.read','project',pid,pid,p.creator_id,p.classification,ch,ai)
  WHEN 'costs' THEN can_read_cost(oid,uid,i.id,(i.facts->'head'->>'current_revision_id')::uuid,ch,ai)
  WHEN 'payments' THEN can_read_payment(oid,uid,i.id,ch,ai)
  WHEN 'contracts' THEN CASE WHEN i.facts->'head'->>'active_revision_id' IS NULL THEN
   can_access(oid,uid,'contract.read','contract',i.id,pid,(i.facts->'head'->>'creator_id')::uuid,i.facts->'head'->>'classification',ch,ai)
   ELSE COALESCE(can_read_contract_closure(oid,uid,i.id,ch,ai),false) END
  WHEN 'purchases' THEN COALESCE(can_read_purchase_completion(oid,uid,i.id,ch,ai),false)
  WHEN 'subscriptions' THEN can_read_subscription(oid,uid,i.id,ch,ai)
  ELSE can_access(oid,uid,k.action,k.rtype,i.id,pid,COALESCE((i.facts->'head'->>'creator_id')::uuid,(i.facts->>'creator_id')::uuid,p.creator_id),COALESCE(i.facts->'head'->>'classification',i.facts->>'classification',p.classification),ch,ai) END)
 AND (k.kind<>'changes' OR can_access(oid,uid,'budget.read','project',pid,pid,p.creator_id,p.classification,ch,ai))
 FROM kinds k JOIN projects p ON p.org_id=oid AND p.id=pid
$$;
CREATE TABLE project_archives (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,project_id uuid NOT NULL,project_version bigint NOT NULL CHECK(project_version>1),
 basis jsonb NOT NULL CHECK(jsonb_typeof(basis)='object'),fingerprint text NOT NULL CHECK(fingerprint~'^[a-f0-9]{32}$'),
 summary text NOT NULL CHECK(length(btrim(summary)) BETWEEN 1 AND 20000),lessons_learned text NOT NULL CHECK(length(btrim(lessons_learned)) BETWEEN 1 AND 20000),follow_up text NOT NULL CHECK(length(btrim(follow_up)) BETWEEN 1 AND 20000),
 business_verified boolean NOT NULL CHECK(business_verified),financial_verified boolean NOT NULL CHECK(financial_verified),
 evidence_count integer NOT NULL CHECK(evidence_count BETWEEN 0 AND 20),actor_id uuid NOT NULL,session_id uuid NOT NULL,mfa_verified_at timestamptz NOT NULL,
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,id),UNIQUE(org_id,project_id,project_version),FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id)
);
CREATE TABLE project_archive_files (org_id uuid NOT NULL,archive_id uuid NOT NULL,version_id uuid NOT NULL,PRIMARY KEY(org_id,archive_id,version_id),FOREIGN KEY(org_id,archive_id) REFERENCES project_archives(org_id,id),FOREIGN KEY(org_id,version_id) REFERENCES file_versions(org_id,id));
CREATE FUNCTION prepare_project_archive() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p projects;s sessions;u users;b jsonb;BEGIN
 SELECT * INTO STRICT p FROM projects WHERE org_id=NEW.org_id AND id=NEW.project_id FOR UPDATE;
 IF p.state<>'closing' OR p.version+1<>NEW.project_version THEN RAISE EXCEPTION 'archive requires the exact closing project' USING ERRCODE='23514'; END IF;
 SELECT * INTO s FROM sessions WHERE org_id=NEW.org_id AND id=NEW.session_id FOR SHARE;
 SELECT * INTO u FROM users WHERE org_id=NEW.org_id AND id=NEW.actor_id;
 IF s.id IS NULL OR s.user_id<>NEW.actor_id OR s.device_type NOT IN('web','android','wechat','douyin') OR s.revoked_at IS NOT NULL OR s.expires_at<=clock_timestamp() OR u.id IS NULL OR u.status<>'active' OR u.auth_version<>s.auth_version OR NEW.mfa_verified_at IS DISTINCT FROM s.mfa_verified_at OR s.mfa_verified_at IS NULL OR s.mfa_verified_at<clock_timestamp()-interval '10 minutes' OR s.mfa_verified_at>clock_timestamp() THEN RAISE EXCEPTION 'archive needs actual recent interactive MFA' USING ERRCODE='23514'; END IF;
 IF NOT can_access(NEW.org_id,NEW.actor_id,'project.archive','project',p.id,p.id,p.creator_id,p.classification,s.device_type,false) OR EXISTS(SELECT 1 FROM project_closeout_readable_groups(NEW.org_id,NEW.actor_id,p.id,s.device_type,false) WHERE NOT readable) THEN RAISE EXCEPTION 'archive needs complete current business authority' USING ERRCODE='23514'; END IF;
 b:=project_closeout_snapshot(NEW.org_id,p.id);
 IF NEW.basis IS DISTINCT FROM b OR NEW.fingerprint IS DISTINCT FROM md5(b::text) OR jsonb_array_length(b->'items')>5000 OR EXISTS(SELECT 1 FROM jsonb_array_elements(b->'items')i WHERE (i->>'blocking')::boolean) THEN RAISE EXCEPTION 'archive basis is stale, incomplete or blocked' USING ERRCODE='23514'; END IF;
 RETURN NEW;END;$$;
CREATE TRIGGER project_archive_prepare BEFORE INSERT ON project_archives FOR EACH ROW EXECUTE FUNCTION prepare_project_archive();
CREATE FUNCTION guard_project_archive_state() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.state='closing' AND OLD.state<>'closing' AND EXISTS(SELECT 1 FROM project_closeout_items(OLD.org_id,OLD.id) WHERE blocking) THEN RAISE EXCEPTION 'closing project has unresolved business or financial items' USING ERRCODE='23514'; END IF;
 IF NEW.state='archived' AND OLD.state<>'archived' AND NOT EXISTS(SELECT 1 FROM project_archives a WHERE a.org_id=NEW.org_id AND a.project_id=NEW.id AND a.project_version=NEW.version AND (a.basis->'project'->>'version')::bigint=OLD.version AND a.basis=project_closeout_snapshot(OLD.org_id,OLD.id)) THEN RAISE EXCEPTION 'project archive requires an exact immutable review' USING ERRCODE='23514'; END IF;RETURN NEW;END;$$;
CREATE TRIGGER project_archive_state BEFORE UPDATE ON projects FOR EACH ROW EXECUTE FUNCTION guard_project_archive_state();
CREATE FUNCTION validate_project_archive() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM projects p WHERE p.org_id=NEW.org_id AND p.id=NEW.project_id AND p.version=NEW.project_version AND p.state='archived'
 AND (to_jsonb(p)-'state'-'version'-'updated_at')=((NEW.basis->'project')-'state'-'version'-'updated_at') AND (project_closeout_snapshot(p.org_id,p.id)->'items')=NEW.basis->'items') OR (SELECT count(*) FROM project_archive_files f WHERE f.org_id=NEW.org_id AND f.archive_id=NEW.id)<>NEW.evidence_count THEN RAISE EXCEPTION 'archive, exact facts and fixed evidence must commit together' USING ERRCODE='23514'; END IF;RETURN NULL;END;$$;
CREATE CONSTRAINT TRIGGER project_archive_consistent AFTER INSERT ON project_archives DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_project_archive();
ALTER TABLE file_version_references DROP CONSTRAINT file_version_references_owner_type_check;
ALTER TABLE file_version_references ADD CHECK(owner_type IN('knowledge_revision','comment','approval_snapshot','contract_revision','contract_signature','purchase_revision','purchase_order','purchase_receipt','claim_revision','cost_revision','payment_revision','subscription_revision','subscription_period_event','contract_finance_event','contract_closure','purchase_variance_revision','purchase_completion','deliverable','project_change_revision','project_archive','recovery_set'));
CREATE FUNCTION pin_project_archive_file() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM project_archives a JOIN projects p ON p.org_id=a.org_id AND p.id=a.project_id WHERE a.org_id=NEW.org_id AND a.id=NEW.archive_id AND p.state='closing' AND p.version+1=a.project_version) THEN RAISE EXCEPTION 'archive evidence is fixed in its original transaction' USING ERRCODE='23514'; END IF;
 INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id) SELECT gen_random_uuid(),NEW.org_id,file_id,id,'project_archive',NEW.archive_id FROM file_versions WHERE org_id=NEW.org_id AND id=NEW.version_id;RETURN NEW;END;$$;
CREATE TRIGGER project_archive_pin AFTER INSERT ON project_archive_files FOR EACH ROW EXECUTE FUNCTION pin_project_archive_file();
CREATE TRIGGER project_archive_immutable BEFORE UPDATE OR DELETE ON project_archives FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER project_archive_files_immutable BEFORE UPDATE OR DELETE ON project_archive_files FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
INSERT INTO roles(id,org_id,key,name,requires_mfa) SELECT gen_random_uuid(),o.id,r.key,r.name,r.mfa FROM organizations o CROSS JOIN (VALUES('project_closeout_reader','项目结项核查查阅人',false),('project_archiver','项目人工归档人',true))r(key,name,mfa) ON CONFLICT(org_id,key) DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action) SELECT r.org_id,r.id,a.action FROM roles r CROSS JOIN (VALUES('project.read'),('task.read'),('milestone.read'),('deliverable.read'),('risk.read'),('issue.read'),('change.read'),('budget.read'),('contract.read'),('purchase.read'),('purchase_variance.read'),('claim.read'),('cost.read'),('payment.read'),('subscription.read'),('party.read'),('file.read'),('folder.read'))a(action) WHERE r.key IN('project_closeout_reader','project_archiver') ON CONFLICT DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,'project.archive' FROM roles WHERE key='project_archiver' ON CONFLICT DO NOTHING;
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN GRANT SELECT,INSERT ON project_archives,project_archive_files TO copypaste_app;REVOKE UPDATE,DELETE ON project_archives,project_archive_files FROM copypaste_app;END IF;END;$$;
