-- Human-reviewed realized differences on fixed remaining payment allocations.
-- This independent ledger never changes expense, cash or allocation amounts.
CREATE TABLE payment_fx_journals (
 id uuid PRIMARY KEY,org_id uuid NOT NULL REFERENCES organizations(id),
 payment_id uuid NOT NULL,version bigint NOT NULL CHECK(version>0),
 allocation_id uuid NOT NULL,cost_id uuid NOT NULL,fx_book_id uuid NOT NULL,
 basis text NOT NULL CHECK(octet_length(basis) BETWEEN 1 AND 131072 AND jsonb_typeof(basis::jsonb)='object'),
 basis_sha256 text NOT NULL CHECK(basis_sha256=encode(sha256(convert_to(basis,'UTF8')),'hex')),
 recognized_on date NOT NULL,actor_id uuid NOT NULL,session_id uuid NOT NULL,
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,id),UNIQUE(org_id,payment_id,version),
 FOREIGN KEY(org_id,payment_id) REFERENCES payment_records(org_id,id),
 FOREIGN KEY(org_id,allocation_id) REFERENCES payment_allocations(org_id,id),
 FOREIGN KEY(org_id,cost_id) REFERENCES actual_cost_entries(org_id,id),
 FOREIGN KEY(org_id,fx_book_id) REFERENCES finance_fx_books(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id)
);
CREATE TABLE payment_fx_journal_revocations (
 id uuid PRIMARY KEY,org_id uuid NOT NULL REFERENCES organizations(id),payment_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0),journal_id uuid NOT NULL,actor_id uuid NOT NULL,session_id uuid NOT NULL,
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,journal_id),UNIQUE(org_id,payment_id,version),
 FOREIGN KEY(org_id,journal_id) REFERENCES payment_fx_journals(org_id,id),
 FOREIGN KEY(org_id,payment_id) REFERENCES payment_records(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id)
);
CREATE INDEX payment_fx_journal_history ON payment_fx_journals(org_id,payment_id,version);
CREATE INDEX payment_fx_journal_root ON payment_fx_journals(org_id,allocation_id);
CREATE FUNCTION payment_fx_journal_version(o uuid,p uuid) RETURNS bigint LANGUAGE sql STABLE AS $$
 SELECT coalesce(max(version),0) FROM (
 SELECT version FROM payment_fx_journals WHERE org_id=o AND payment_id=p
 UNION ALL SELECT version FROM payment_fx_journal_revocations WHERE org_id=o AND payment_id=p) x
$$;
CREATE FUNCTION payment_fx_journal_rate(b jsonb,c text,d date) RETURNS numeric LANGUAGE sql IMMUTABLE AS $$
 SELECT CASE WHEN c=b->>'base_currency' THEN 1 ELSE
 (SELECT (r->>'rate')::numeric FROM jsonb_array_elements(b->'rates') r
 WHERE r->>'currency'=c AND (r->>'valid_from')::date<=d AND d<(r->>'valid_to')::date) END
$$;
CREATE FUNCTION validate_payment_fx_journal() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE b jsonb;p payment_records;c actual_cost_entries;pr payment_revisions;cr cost_revisions;a payment_allocations;
 f finance_fx_books;s sessions;remaining numeric;cost_remaining numeric;exponent integer;carry numeric;settled numeric;rate_c numeric;rate_p numeric;
BEGIN
 PERFORM id FROM organizations WHERE id=NEW.org_id FOR UPDATE;
 SELECT * INTO s FROM sessions WHERE org_id=NEW.org_id AND id=NEW.session_id;
 IF NEW.version<>payment_fx_journal_version(NEW.org_id,NEW.payment_id)+1 OR s.user_id IS DISTINCT FROM NEW.actor_id
 OR s.device_type NOT IN ('web','wechat','douyin','android') OR s.revoked_at IS NOT NULL OR s.expires_at<=clock_timestamp()
 OR s.mfa_verified_at IS NULL OR s.mfa_verified_at<=clock_timestamp()-interval '10 minutes'
 OR s.mfa_verified_at>clock_timestamp()+interval '1 minute'
 OR NOT EXISTS(SELECT 1 FROM users u WHERE u.org_id=NEW.org_id AND u.id=NEW.actor_id AND u.status='active' AND u.auth_version=s.auth_version) THEN
  RAISE EXCEPTION 'FX journal requires exact next version and current human MFA session' USING ERRCODE='23514';
 END IF;
 SELECT * INTO p FROM payment_records WHERE org_id=NEW.org_id AND id=NEW.payment_id;
 IF NOT can_read_payment(NEW.org_id,NEW.actor_id,p.id,s.device_type,false)
 OR NOT can_access(NEW.org_id,NEW.actor_id,'payment.record','payment',p.id,p.project_id,p.creator_id,p.classification,s.device_type,false) THEN
  RAISE EXCEPTION 'FX journal requires current ordinary payment authority' USING ERRCODE='23514';
 END IF;
 IF TG_TABLE_NAME='payment_fx_journal_revocations' THEN
  IF NOT EXISTS(SELECT 1 FROM payment_fx_journals j WHERE j.org_id=NEW.org_id AND j.id=NEW.journal_id AND j.payment_id=NEW.payment_id) THEN
   RAISE EXCEPTION 'FX revocation must refer to its original journal' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
 END IF;
 b:=NEW.basis::jsonb;
 SELECT * INTO pr FROM payment_revisions WHERE org_id=NEW.org_id AND id=p.current_revision_id;
 SELECT * INTO c FROM actual_cost_entries WHERE org_id=NEW.org_id AND id=NEW.cost_id;
 SELECT * INTO cr FROM cost_revisions WHERE org_id=NEW.org_id AND id=c.current_revision_id;
 SELECT * INTO a FROM payment_allocations WHERE org_id=NEW.org_id AND id=NEW.allocation_id;
 SELECT * INTO f FROM finance_fx_books WHERE org_id=NEW.org_id AND id=NEW.fx_book_id;
 SELECT coalesce(sum(effect*amount),0),coalesce(sum(effect*cost_amount),0) INTO remaining,cost_remaining
 FROM payment_allocations WHERE org_id=NEW.org_id AND root_id=NEW.allocation_id;
 SELECT d.exponent INTO exponent FROM currency_definitions d WHERE d.code=f.base_currency;
 rate_c:=payment_fx_journal_rate(f.content::jsonb,cr.currency,cr.occurred_on);
 rate_p:=payment_fx_journal_rate(f.content::jsonb,pr.currency,pr.paid_on);
 carry:=round(cost_remaining*rate_c,exponent);settled:=round(remaining*rate_p,exponent);
 IF p.state IS DISTINCT FROM 'recorded' OR p.record_kind IS DISTINCT FROM 'payment' OR pr.direction IS DISTINCT FROM 'outgoing'
 OR c.state IS DISTINCT FROM 'confirmed' OR c.entry_kind IS DISTINCT FROM 'expense'
 OR a.id IS NULL OR a.kind<>'allocation' OR a.root_id<>a.id OR a.effect<>1 OR a.payment_id<>p.id OR a.cost_id<>c.id
 OR pr.currency=cr.currency OR p.project_id IS DISTINCT FROM c.project_id OR p.classification IS DISTINCT FROM c.classification
 OR remaining<=0 OR cost_remaining<=0 OR rate_c IS NULL OR rate_p IS NULL OR NEW.recognized_on<pr.paid_on
 OR NOT can_read_cost(NEW.org_id,NEW.actor_id,c.id,c.current_revision_id,s.device_type,false)
 OR NOT can_access(NEW.org_id,NEW.actor_id,'cost.record','cost',c.id,c.project_id,c.creator_id,c.classification,s.device_type,false)
 OR NOT can_access(NEW.org_id,NEW.actor_id,'report.read','report',NULL,p.project_id,NEW.actor_id,'internal',s.device_type,false)
 OR (b->>'method'='reviewed_remaining_allocation_v1' AND b->'payment'->>'id'=p.id::text
 AND (b->'payment'->>'version')::bigint=p.version AND b->'payment'->>'revision_id'=pr.id::text AND b->'payment'->>'content_hash'=pr.content_hash
 AND b->'cost'->>'id'=c.id::text AND (b->'cost'->>'version')::bigint=c.version AND b->'cost'->>'revision_id'=cr.id::text AND b->'cost'->>'content_hash'=cr.content_hash
 AND b->'allocation'->>'id'=a.id::text AND b->'allocation'->>'payment_id'=p.id::text AND b->'allocation'->>'cost_id'=c.id::text
 AND (b->'allocation'->>'remaining_amount')::numeric=remaining AND (b->'allocation'->>'remaining_cost_amount')::numeric=cost_remaining
 AND b->'book'->>'id'=f.id::text AND b->'book'->>'content_sha256'=f.content_sha256 AND b->'book'->>'base_currency'=f.base_currency
 AND b->>'cost_date'=cr.occurred_on::text AND b->>'payment_date'=pr.paid_on::text
 AND (b->'carrying'->>'base_amount')::numeric=carry AND (b->'settled'->>'base_amount')::numeric=settled
 AND (b->>'gain')::numeric=greatest(carry-settled,0) AND (b->>'loss')::numeric=greatest(settled-carry,0)) IS NOT TRUE
 OR EXISTS(SELECT 1 FROM payment_fx_journals j WHERE j.org_id=NEW.org_id AND j.allocation_id=a.id AND NOT EXISTS(
 SELECT 1 FROM payment_fx_journal_revocations r WHERE r.org_id=j.org_id AND r.journal_id=j.id)) THEN
  RAISE EXCEPTION 'FX journal requires exact live facts, fixed complete rates and one active root' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER fx_journal_evidence BEFORE INSERT ON payment_fx_journals FOR EACH ROW EXECUTE FUNCTION validate_payment_fx_journal();
CREATE TRIGGER fx_journal_revocation_evidence BEFORE INSERT ON payment_fx_journal_revocations FOR EACH ROW EXECUTE FUNCTION validate_payment_fx_journal();
CREATE TRIGGER fx_journal_immutable BEFORE UPDATE OR DELETE ON payment_fx_journals FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER fx_journal_revocation_immutable BEFORE UPDATE OR DELETE ON payment_fx_journal_revocations FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON payment_fx_journals,payment_fx_journal_revocations TO copypaste_app;
 REVOKE UPDATE,DELETE ON payment_fx_journals,payment_fx_journal_revocations FROM copypaste_app;
END IF;END $$;
