-- Human classification of existing cash/expense facts. No monetary rows change.
CREATE TABLE payment_fee_links (
 id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
 payment_id uuid NOT NULL, version bigint NOT NULL CHECK(version>0),
 fee_payment_id uuid NOT NULL, cost_id uuid NOT NULL, allocation_id uuid NOT NULL,
 basis text NOT NULL CHECK(octet_length(basis) BETWEEN 1 AND 131072 AND jsonb_typeof(basis::jsonb)='object'),
 basis_sha256 text NOT NULL CHECK(basis_sha256=encode(sha256(convert_to(basis,'UTF8')),'hex')),
 actor_id uuid NOT NULL, session_id uuid NOT NULL,
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,id), UNIQUE(org_id,payment_id,version),
 FOREIGN KEY(org_id,payment_id) REFERENCES payment_records(org_id,id),
 FOREIGN KEY(org_id,fee_payment_id) REFERENCES payment_records(org_id,id),
 FOREIGN KEY(org_id,cost_id) REFERENCES actual_cost_entries(org_id,id),
 FOREIGN KEY(org_id,allocation_id) REFERENCES payment_allocations(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id)
);
CREATE TABLE payment_fee_revocations (
 id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
 payment_id uuid NOT NULL, version bigint NOT NULL CHECK(version>0), link_id uuid NOT NULL,
 actor_id uuid NOT NULL, session_id uuid NOT NULL,
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,link_id), UNIQUE(org_id,payment_id,version),
 FOREIGN KEY(org_id,link_id) REFERENCES payment_fee_links(org_id,id),
 FOREIGN KEY(org_id,payment_id) REFERENCES payment_records(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id)
);
CREATE INDEX payment_fee_history ON payment_fee_links(org_id,payment_id,created_at DESC,id DESC);
CREATE INDEX payment_fee_allocation ON payment_fee_links(org_id,allocation_id);
CREATE FUNCTION payment_fee_version(o uuid,p uuid) RETURNS bigint LANGUAGE sql STABLE AS $$
 SELECT COALESCE(max(version),0) FROM (
  SELECT version FROM payment_fee_links WHERE org_id=o AND payment_id=p
  UNION ALL SELECT version FROM payment_fee_revocations WHERE org_id=o AND payment_id=p
 ) v
$$;
CREATE TRIGGER payment_fee_link_immutable BEFORE UPDATE OR DELETE ON payment_fee_links
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER payment_fee_revocation_immutable BEFORE UPDATE OR DELETE ON payment_fee_revocations
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION validate_payment_fee_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE b jsonb; p payment_records; q payment_records; c actual_cost_entries;
 pr payment_revisions; qr payment_revisions; cr cost_revisions; a payment_allocations;
 remaining numeric; cost_remaining numeric;
BEGIN
 -- Same lock as the application coordinator; concurrent distinct principals
 -- cannot classify the same live root twice through direct inserts either.
 PERFORM id FROM organizations WHERE id=NEW.org_id FOR UPDATE;
 IF NEW.version<>payment_fee_version(NEW.org_id,NEW.payment_id)+1
  OR NOT EXISTS(SELECT 1 FROM sessions s JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id
   WHERE s.org_id=NEW.org_id AND s.id=NEW.session_id AND s.user_id=NEW.actor_id
   AND s.device_type IN ('web','wechat','douyin','android') AND s.revoked_at IS NULL
   AND s.expires_at>clock_timestamp() AND u.status='active' AND s.auth_version=u.auth_version
   AND s.mfa_verified_at>clock_timestamp()-interval '10 minutes'
   AND s.mfa_verified_at<=clock_timestamp()+interval '1 minute') THEN
  RAISE EXCEPTION 'fee evidence requires its current human MFA session and next version' USING ERRCODE='23514';
 END IF;
 IF TG_TABLE_NAME='payment_fee_revocations' THEN
  IF NOT EXISTS(SELECT 1 FROM payment_fee_links l WHERE l.org_id=NEW.org_id AND l.id=NEW.link_id AND l.payment_id=NEW.payment_id) THEN
   RAISE EXCEPTION 'fee revocation requires the exact original link' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
 END IF;
 b:=NEW.basis::jsonb;
 SELECT * INTO p FROM payment_records WHERE org_id=NEW.org_id AND id=NEW.payment_id;
 SELECT * INTO q FROM payment_records WHERE org_id=NEW.org_id AND id=NEW.fee_payment_id;
 SELECT * INTO c FROM actual_cost_entries WHERE org_id=NEW.org_id AND id=NEW.cost_id;
 SELECT * INTO pr FROM payment_revisions WHERE org_id=p.org_id AND id=p.current_revision_id;
 SELECT * INTO qr FROM payment_revisions WHERE org_id=q.org_id AND id=q.current_revision_id;
 SELECT * INTO cr FROM cost_revisions WHERE org_id=c.org_id AND id=c.current_revision_id;
 SELECT * INTO a FROM payment_allocations WHERE org_id=NEW.org_id AND id=NEW.allocation_id;
 SELECT COALESCE(sum(effect*amount),0),COALESCE(sum(effect*cost_amount),0) INTO remaining,cost_remaining
  FROM payment_allocations WHERE org_id=NEW.org_id AND root_id=NEW.allocation_id;
 IF p.state IS DISTINCT FROM 'recorded' OR q.state IS DISTINCT FROM 'recorded'
  OR p.record_kind IS DISTINCT FROM 'payment' OR q.record_kind IS DISTINCT FROM 'payment'
  OR pr.direction IS DISTINCT FROM 'outgoing' OR qr.direction IS DISTINCT FROM 'outgoing'
  OR c.state IS DISTINCT FROM 'confirmed' OR c.entry_kind IS DISTINCT FROM 'expense'
  OR p.project_id IS DISTINCT FROM q.project_id OR p.project_id IS DISTINCT FROM c.project_id
  OR p.classification IS DISTINCT FROM q.classification OR p.classification IS DISTINCT FROM c.classification
  OR a.id IS NULL OR a.kind<>'allocation' OR a.effect<>1 OR a.root_id<>a.id OR a.payment_id<>q.id OR a.cost_id<>c.id
  OR remaining<=0 OR cost_remaining<=0
  OR (b->'payment'->>'id'=p.id::text AND (b->'payment'->>'version')::bigint=p.version
   AND b->'payment'->>'revision_id'=pr.id::text AND b->'payment'->>'content_hash'=pr.content_hash
   AND b->'fee_payment'->>'id'=q.id::text AND (b->'fee_payment'->>'version')::bigint=q.version
   AND b->'fee_payment'->>'revision_id'=qr.id::text AND b->'fee_payment'->>'content_hash'=qr.content_hash
   AND b->'cost'->>'id'=c.id::text AND (b->'cost'->>'version')::bigint=c.version
   AND b->'cost'->>'revision_id'=cr.id::text AND b->'cost'->>'content_hash'=cr.content_hash
   AND b->'allocation'->>'id'=a.id::text AND b->'allocation'->>'payment_id'=a.payment_id::text
   AND b->'allocation'->>'cost_id'=a.cost_id::text AND b->'allocation'->>'cost_revision_id'=a.cost_revision_id::text
   AND (b->'allocation'->>'amount')::numeric=a.amount AND (b->'allocation'->>'cost_amount')::numeric=a.cost_amount
   AND (b->'allocation'->>'remaining_amount')::numeric=remaining
   AND (b->'allocation'->>'remaining_cost_amount')::numeric=cost_remaining) IS NOT TRUE
  OR EXISTS(SELECT 1 FROM payment_fee_links l WHERE l.org_id=NEW.org_id AND l.allocation_id=a.id
   AND NOT EXISTS(SELECT 1 FROM payment_fee_revocations r WHERE r.org_id=l.org_id AND r.link_id=l.id)) THEN
  RAISE EXCEPTION 'fee evidence requires exact live cash/expense/root facts and an unclassified allocation' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER payment_fee_link_evidence BEFORE INSERT ON payment_fee_links
 FOR EACH ROW EXECUTE FUNCTION validate_payment_fee_evidence();
CREATE TRIGGER payment_fee_revocation_evidence BEFORE INSERT ON payment_fee_revocations
 FOR EACH ROW EXECUTE FUNCTION validate_payment_fee_evidence();
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON payment_fee_links,payment_fee_revocations TO copypaste_app;
 REVOKE UPDATE,DELETE ON payment_fee_links,payment_fee_revocations FROM copypaste_app;
END IF; END; $$;
