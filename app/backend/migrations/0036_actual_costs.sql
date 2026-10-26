-- Canonical incurred costs; forecasts and cash remain independent facts.
CREATE TABLE actual_cost_entries (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), source_key text NOT NULL,
    project_id uuid, creator_id uuid NOT NULL, classification text NOT NULL CHECK(classification IN ('internal','project_restricted','confidential')),
    entry_kind text NOT NULL CHECK(entry_kind IN ('expense','reversal')), reverses_id uuid,
    state text NOT NULL DEFAULT 'draft' CHECK(state IN ('draft','confirmed','reversed')),
    current_revision_id uuid NOT NULL, version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,source_key), UNIQUE(org_id,reverses_id),
    FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id), FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),
    FOREIGN KEY(org_id,reverses_id) REFERENCES actual_cost_entries(org_id,id),
    CHECK(source_key~'^[a-z][a-z0-9_.-]{1,31}:[A-Za-z0-9._:/-]{1,166}$'),
    CHECK((entry_kind='reversal')=(reverses_id IS NOT NULL)), CHECK(reverses_id IS NULL OR reverses_id<>id),
    CHECK(entry_kind<>'reversal' OR state='confirmed'), CHECK(classification<>'project_restricted' OR project_id IS NOT NULL)
);
CREATE TABLE cost_revisions (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, cost_id uuid NOT NULL, seq bigint NOT NULL CHECK(seq>0), base_revision_id uuid,
    payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object'), content_hash text NOT NULL CHECK(content_hash~'^[0-9a-f]{64}$'),
    title text GENERATED ALWAYS AS (payload->>'title') STORED NOT NULL CHECK(length(btrim(title)) BETWEEN 1 AND 200),
    occurred_on date,
    currency text GENERATED ALWAYS AS (payload->>'currency') STORED REFERENCES currency_definitions(code),
    amount numeric(20,6) GENERATED ALWAYS AS ((payload->>'amount')::numeric) STORED CHECK(amount>=0),
    category text GENERATED ALWAYS AS (payload->>'category') STORED NOT NULL CHECK(length(category)<=50),
    counterparty_id uuid GENERATED ALWAYS AS ((payload->'counterparty'->>'party_id')::uuid) STORED,
    counterparty_revision_id uuid GENERATED ALWAYS AS ((payload->'counterparty'->>'revision_id')::uuid) STORED,
    author_id uuid NOT NULL, reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200), created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,cost_id,id), UNIQUE(org_id,cost_id,seq),
    FOREIGN KEY(org_id,cost_id) REFERENCES actual_cost_entries(org_id,id),
    FOREIGN KEY(org_id,cost_id,base_revision_id) REFERENCES cost_revisions(org_id,cost_id,id),
    FOREIGN KEY(org_id,author_id) REFERENCES users(org_id,id),
    FOREIGN KEY(org_id,counterparty_id,counterparty_revision_id) REFERENCES party_revisions(org_id,party_id,id),
    CHECK((seq=1)=(base_revision_id IS NULL)), CHECK(amount IS NULL OR currency IS NOT NULL),
    CHECK((counterparty_id IS NULL)=(counterparty_revision_id IS NULL)),
    CHECK(jsonb_typeof(payload->'allocations')='array' AND jsonb_array_length(payload->'allocations')<=100),
    CHECK(jsonb_typeof(payload->'evidence_version_ids')='array' AND jsonb_array_length(payload->'evidence_version_ids')<=20)
);
ALTER TABLE actual_cost_entries ADD FOREIGN KEY(org_id,id,current_revision_id) REFERENCES cost_revisions(org_id,cost_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE cost_allocations (
    org_id uuid NOT NULL, cost_id uuid NOT NULL, revision_id uuid NOT NULL, position integer NOT NULL CHECK(position BETWEEN 1 AND 100),
    project_id uuid, amount numeric(20,6) NOT NULL CHECK(amount>0), PRIMARY KEY(org_id,revision_id,position),
    FOREIGN KEY(org_id,cost_id,revision_id) REFERENCES cost_revisions(org_id,cost_id,id), FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id)
);
CREATE UNIQUE INDEX cost_allocation_scope ON cost_allocations(org_id,revision_id,COALESCE(project_id,'00000000-0000-0000-0000-000000000000'));
CREATE TABLE cost_revision_files (
    org_id uuid NOT NULL, revision_id uuid NOT NULL, version_id uuid NOT NULL, PRIMARY KEY(org_id,revision_id,version_id),
    FOREIGN KEY(org_id,revision_id) REFERENCES cost_revisions(org_id,id), FOREIGN KEY(org_id,version_id) REFERENCES file_versions(org_id,id)
);
CREATE TABLE cost_links (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, cost_id uuid NOT NULL, source_type text NOT NULL CHECK(source_type IN ('purchase','claim','contract')),
    source_id uuid NOT NULL, revision_id uuid NOT NULL, relation text NOT NULL CHECK(relation IN ('incurred_for','reimbursed_via','contractual_basis')),
    actor_id uuid NOT NULL, reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200), created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,cost_id,source_type,source_id), FOREIGN KEY(org_id,cost_id) REFERENCES actual_cost_entries(org_id,id),
    FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),
    CHECK((source_type='purchase' AND relation='incurred_for') OR (source_type='claim' AND relation='reimbursed_via') OR (source_type='contract' AND relation='contractual_basis'))
);
CREATE TABLE cost_events (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, cost_id uuid NOT NULL, cost_version bigint NOT NULL CHECK(cost_version>0),
    revision_id uuid NOT NULL, kind text NOT NULL CHECK(kind IN ('draft_created','draft_revised','confirmed','reversed','link_added','reversal_created')),
    state text NOT NULL CHECK(state IN ('draft','confirmed','reversed')), link_id uuid,
    actor_id uuid NOT NULL, session_id uuid NOT NULL, mfa_verified_at timestamptz,
    reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200), created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,cost_id,cost_version), FOREIGN KEY(org_id,cost_id,revision_id) REFERENCES cost_revisions(org_id,cost_id,id),
    FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id), FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id),
    FOREIGN KEY(org_id,link_id) REFERENCES cost_links(org_id,id),
    CHECK((kind='link_added')=(link_id IS NOT NULL)),
    CHECK(kind IN ('draft_created','draft_revised') OR mfa_verified_at IS NOT NULL)
);
-- Validate the session at insertion, preserving its historical MFA timestamp.
CREATE FUNCTION verify_cost_event_session() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE sess sessions; account users;
BEGIN
    SELECT * INTO sess FROM sessions WHERE org_id=NEW.org_id AND id=NEW.session_id FOR SHARE;
    SELECT * INTO account FROM users WHERE org_id=NEW.org_id AND id=NEW.actor_id;
    IF sess.id IS NULL OR sess.user_id<>NEW.actor_id OR sess.revoked_at IS NOT NULL OR sess.expires_at<=clock_timestamp()
        OR account.id IS NULL OR account.status<>'active' OR account.auth_version<>sess.auth_version
        OR NEW.mfa_verified_at IS DISTINCT FROM sess.mfa_verified_at
        OR (NEW.kind NOT IN ('draft_created','draft_revised') AND (sess.mfa_verified_at IS NULL OR sess.mfa_verified_at<clock_timestamp()-interval '10 minutes' OR sess.mfa_verified_at>clock_timestamp())) THEN
        RAISE EXCEPTION 'cost event requires its actual active human session and recent MFA' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER cost_event_session BEFORE INSERT ON cost_events FOR EACH ROW EXECUTE FUNCTION verify_cost_event_session();
-- Serializing the shared source also protects totals for direct SQL writers.
CREATE FUNCTION lock_cost_link_source() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.source_type='contract' THEN
        PERFORM id FROM contracts WHERE org_id=NEW.org_id AND id=NEW.source_id FOR UPDATE;
    ELSE
        PERFORM id FROM spend_documents WHERE org_id=NEW.org_id AND id=NEW.source_id AND kind=NEW.source_type FOR UPDATE;
    END IF;
    IF NOT FOUND THEN RAISE EXCEPTION 'cost link source missing' USING ERRCODE='23514'; END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER cost_link_source_lock BEFORE INSERT ON cost_links FOR EACH ROW EXECUTE FUNCTION lock_cost_link_source();
CREATE INDEX cost_list ON actual_cost_entries(org_id,updated_at DESC,id DESC);
CREATE INDEX cost_period ON cost_revisions(org_id,occurred_on,currency);
CREATE INDEX cost_links_source ON cost_links(org_id,source_type,source_id,cost_id);
CREATE UNIQUE INDEX cost_event_link_once ON cost_events(org_id,link_id) WHERE link_id IS NOT NULL;
ALTER TABLE file_version_references DROP CONSTRAINT file_version_references_owner_type_check;
ALTER TABLE file_version_references ADD CHECK(owner_type IN ('knowledge_revision','comment','approval_snapshot','contract_revision','contract_signature','purchase_revision','purchase_order','purchase_receipt','claim_revision','cost_revision','deliverable','recovery_set'));
CREATE FUNCTION register_cost_file() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id)
        SELECT gen_random_uuid(),NEW.org_id,file_id,id,'cost_revision',NEW.revision_id FROM file_versions WHERE org_id=NEW.org_id AND id=NEW.version_id;
    RETURN NEW;
END; $$;
CREATE TRIGGER cost_file_reference AFTER INSERT ON cost_revision_files FOR EACH ROW EXECUTE FUNCTION register_cost_file();
CREATE FUNCTION can_read_cost(p_org uuid,p_user uuid,p_cost uuid,p_revision uuid,p_channel text,p_ai boolean) RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT EXISTS(SELECT 1 FROM actual_cost_entries c JOIN cost_revisions r ON r.org_id=c.org_id AND r.cost_id=c.id AND r.id=p_revision
        WHERE c.org_id=p_org AND c.id=p_cost AND can_access(c.org_id,p_user,'cost.read','cost',c.id,c.project_id,c.creator_id,c.classification,p_channel,p_ai)
        AND NOT EXISTS(SELECT 1 FROM cost_allocations a JOIN projects p ON p.org_id=a.org_id AND p.id=a.project_id
            WHERE a.org_id=c.org_id AND a.revision_id=r.id AND NOT can_access(p.org_id,p_user,'project.read','project',p.id,p.id,p.creator_id,p.classification,p_channel,p_ai)))
$$;
CREATE FUNCTION preserve_cost_head() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(NEW.id,NEW.org_id,NEW.source_key,NEW.project_id,NEW.creator_id,NEW.classification,NEW.entry_kind,NEW.reverses_id,NEW.created_at)
        IS DISTINCT FROM ROW(OLD.id,OLD.org_id,OLD.source_key,OLD.project_id,OLD.creator_id,OLD.classification,OLD.entry_kind,OLD.reverses_id,OLD.created_at)
        OR NEW.version<>OLD.version+1 OR OLD.entry_kind='reversal' OR OLD.state='reversed'
        OR (OLD.state='confirmed' AND (NEW.current_revision_id<>OLD.current_revision_id OR NEW.state NOT IN ('confirmed','reversed')))
        OR (OLD.state='draft' AND (NEW.state NOT IN ('draft','confirmed') OR (NEW.state='confirmed' AND NEW.current_revision_id<>OLD.current_revision_id))) THEN
        RAISE EXCEPTION 'cost identity, confirmed facts and version progression are protected' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER cost_head_identity BEFORE UPDATE ON actual_cost_entries FOR EACH ROW EXECUTE FUNCTION preserve_cost_head();
CREATE TRIGGER cost_head_no_delete BEFORE DELETE ON actual_cost_entries FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION validate_cost() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE cid uuid; oid uuid:=NEW.org_id; c actual_cost_entries; r cost_revisions; e cost_events; original actual_cost_entries; original_revision cost_revisions; link cost_links;
    alloc jsonb; evidence jsonb; exponent integer; source_currency text; source_amount numeric; source_class text; source_project uuid; used numeric;
BEGIN
    IF TG_TABLE_NAME='actual_cost_entries' THEN cid:=NEW.id;
    ELSIF TG_TABLE_NAME='cost_revision_files' THEN SELECT cost_id INTO cid FROM cost_revisions WHERE org_id=oid AND id=NEW.revision_id;
    ELSE cid:=NEW.cost_id; END IF;
    SELECT * INTO STRICT c FROM actual_cost_entries WHERE org_id=oid AND id=cid;
    SELECT * INTO STRICT r FROM cost_revisions WHERE org_id=oid AND cost_id=cid AND id=c.current_revision_id;
    SELECT * INTO e FROM cost_events WHERE org_id=oid AND cost_id=cid ORDER BY cost_version DESC LIMIT 1;
    IF e.id IS NULL OR e.cost_version<>c.version OR e.state<>c.state OR e.revision_id<>c.current_revision_id
        OR (SELECT count(*) FROM cost_events WHERE org_id=oid AND cost_id=cid)<>c.version
        OR r.seq<>(SELECT count(*) FROM cost_revisions WHERE org_id=oid AND cost_id=cid)
        OR r.seq<>(SELECT max(seq) FROM cost_revisions WHERE org_id=oid AND cost_id=cid)
        OR (SELECT count(*) FROM cost_links WHERE org_id=oid AND cost_id=cid)>100 THEN
        RAISE EXCEPTION 'cost head requires a complete immutable journal' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM cost_events x LEFT JOIN cost_events prev ON prev.org_id=x.org_id AND prev.cost_id=x.cost_id AND prev.cost_version=x.cost_version-1
        LEFT JOIN cost_revisions revision ON revision.org_id=x.org_id AND revision.id=x.revision_id
        LEFT JOIN cost_revisions previous_revision ON previous_revision.org_id=prev.org_id AND previous_revision.id=prev.revision_id
        WHERE x.org_id=oid AND x.cost_id=cid AND NOT COALESCE((
            (x.kind='draft_created' AND x.cost_version=1 AND x.state='draft' AND revision.seq=1 AND c.entry_kind='expense') OR
            (x.kind='reversal_created' AND x.cost_version=1 AND x.state='confirmed' AND revision.seq=1 AND c.entry_kind='reversal') OR
            (x.kind='draft_revised' AND x.state='draft' AND prev.state='draft' AND revision.seq=previous_revision.seq+1 AND revision.base_revision_id=prev.revision_id) OR
            (x.kind='confirmed' AND x.state='confirmed' AND prev.state='draft' AND x.revision_id=prev.revision_id) OR
            (x.kind='link_added' AND x.state='confirmed' AND prev.state='confirmed' AND x.revision_id=prev.revision_id AND EXISTS(SELECT 1 FROM cost_links l WHERE l.org_id=x.org_id AND l.id=x.link_id AND l.cost_id=cid)) OR
            (x.kind='reversed' AND x.state='reversed' AND prev.state='confirmed' AND x.revision_id=prev.revision_id AND c.entry_kind='expense')
        ),false)) THEN RAISE EXCEPTION 'cost journal transition is invalid' USING ERRCODE='23514'; END IF;
    FOR r IN SELECT * FROM cost_revisions WHERE org_id=oid AND cost_id=cid LOOP
        SELECT COALESCE(jsonb_agg(jsonb_build_object('project_id',a.project_id,'amount',a.amount::text) ORDER BY position),'[]') INTO alloc FROM cost_allocations a WHERE a.org_id=oid AND a.revision_id=r.id;
        SELECT COALESCE(jsonb_agg(version_id::text ORDER BY version_id::text),'[]') INTO evidence FROM cost_revision_files WHERE org_id=oid AND revision_id=r.id;
        IF alloc IS DISTINCT FROM r.payload->'allocations' OR evidence IS DISTINCT FROM r.payload->'evidence_version_ids'
            OR to_char(r.occurred_on,'YYYY-MM-DD') IS DISTINCT FROM r.payload->>'occurred_on'
            OR length(r.payload->>'notes')>20000 OR (r.seq>1 AND NOT EXISTS(SELECT 1 FROM cost_revisions p WHERE p.org_id=oid AND p.cost_id=cid AND p.id=r.base_revision_id AND p.seq=r.seq-1)) THEN
            RAISE EXCEPTION 'cost normalized facts differ from fixed revision' USING ERRCODE='23514';
        END IF;
        SELECT d.exponent INTO exponent FROM currency_definitions d WHERE d.code=r.currency;
        IF r.amount IS NOT NULL AND r.amount*power(10::numeric,exponent)<>trunc(r.amount*power(10::numeric,exponent))
            OR EXISTS(SELECT 1 FROM cost_allocations a WHERE a.org_id=oid AND a.revision_id=r.id AND (r.currency IS NULL OR a.amount*power(10::numeric,exponent)<>trunc(a.amount*power(10::numeric,exponent)))) THEN
            RAISE EXCEPTION 'cost currency precision is invalid' USING ERRCODE='23514';
        END IF;
        IF EXISTS(SELECT 1 FROM cost_allocations a JOIN projects p ON p.org_id=a.org_id AND p.id=a.project_id WHERE a.org_id=oid AND a.revision_id=r.id AND classification_rank(p.classification)>classification_rank(c.classification)) THEN
            RAISE EXCEPTION 'cost allocation cannot lower project classification' USING ERRCODE='23514';
        END IF;
    END LOOP;
    SELECT * INTO STRICT r FROM cost_revisions WHERE org_id=oid AND id=c.current_revision_id;
    IF c.state<>'draft' AND (r.occurred_on IS NULL OR r.occurred_on>(clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date OR r.currency IS NULL OR r.amount IS NULL OR r.amount<=0 OR btrim(r.category)=''
        OR r.counterparty_id IS NULL OR jsonb_array_length(r.payload->'evidence_version_ids')=0 OR jsonb_array_length(r.payload->'allocations')=0
        OR (SELECT COALESCE(sum(amount),0) FROM cost_allocations WHERE org_id=oid AND revision_id=r.id)<>r.amount) THEN
        RAISE EXCEPTION 'confirmed cost requires complete balanced actual facts' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM projects p WHERE p.org_id=oid AND p.id=c.project_id AND classification_rank(p.classification)>classification_rank(c.classification)) THEN
        RAISE EXCEPTION 'cost cannot lower its home project classification' USING ERRCODE='23514';
    END IF;
    IF c.entry_kind='reversal' THEN
        SELECT * INTO STRICT original FROM actual_cost_entries WHERE org_id=oid AND id=c.reverses_id;
        SELECT * INTO STRICT original_revision FROM cost_revisions WHERE org_id=oid AND id=original.current_revision_id;
        IF original.entry_kind<>'expense' OR original.state<>'reversed' OR c.source_key<>'reversal:'||original.id::text
            OR ROW(c.project_id,c.creator_id,c.classification) IS DISTINCT FROM ROW(original.project_id,original.creator_id,original.classification)
            OR (r.payload-'occurred_on') IS DISTINCT FROM (original_revision.payload-'occurred_on') OR r.occurred_on<original_revision.occurred_on THEN
            RAISE EXCEPTION 'reversal must inherit the full original cost snapshot' USING ERRCODE='23514';
        END IF;
    ELSIF c.state='reversed' AND NOT EXISTS(SELECT 1 FROM actual_cost_entries x WHERE x.org_id=oid AND x.reverses_id=cid AND x.entry_kind='reversal' AND x.state='confirmed') THEN
        RAISE EXCEPTION 'reversed cost needs its compensating entry' USING ERRCODE='23514';
    END IF;
    FOR link IN SELECT * FROM cost_links WHERE org_id=oid AND cost_id=cid LOOP
        IF c.entry_kind<>'expense' OR NOT EXISTS(SELECT 1 FROM cost_events x WHERE x.org_id=oid AND x.cost_id=cid AND x.kind='link_added' AND x.link_id=link.id) THEN
            RAISE EXCEPTION 'cost links need their own immutable confirmation event' USING ERRCODE='23514';
        END IF;
        source_currency:=NULL; source_amount:=NULL;
        IF link.source_type='contract' THEN
            SELECT v.currency,v.amount,d.classification,d.project_id INTO source_currency,source_amount,source_class,source_project FROM contracts d JOIN contract_revisions v ON v.org_id=d.org_id AND v.id=link.revision_id AND v.contract_id=d.id
                WHERE d.org_id=oid AND d.id=link.source_id AND d.approved_revision_id=v.id AND d.active_revision_id=v.id AND v.direction='payable';
        ELSE
            SELECT v.currency,v.amount,d.classification,d.project_id INTO source_currency,source_amount,source_class,source_project FROM spend_documents d JOIN spend_revisions v ON v.org_id=d.org_id AND v.id=link.revision_id AND v.document_id=d.id
                WHERE d.org_id=oid AND d.id=link.source_id AND d.kind=link.source_type AND d.approved_revision_id=v.id;
        END IF;
        SELECT COALESCE(sum(v.amount),0) INTO used FROM cost_links l JOIN actual_cost_entries d ON d.org_id=l.org_id AND d.id=l.cost_id JOIN cost_revisions v ON v.org_id=d.org_id AND v.id=d.current_revision_id
            WHERE l.org_id=oid AND l.source_type=link.source_type AND l.source_id=link.source_id AND d.state='confirmed' AND d.entry_kind='expense';
        IF source_currency IS NULL OR source_currency<>r.currency OR source_amount IS NULL OR used>source_amount
            OR classification_rank(source_class)>classification_rank(c.classification)
            OR (source_project IS NOT NULL AND source_project IS DISTINCT FROM c.project_id AND NOT EXISTS(SELECT 1 FROM cost_allocations a WHERE a.org_id=oid AND a.revision_id=r.id AND a.project_id=source_project)) THEN
            RAISE EXCEPTION 'cost links must respect exact approved currency scope and total' USING ERRCODE='23514';
        END IF;
    END LOOP;
    RETURN NULL;
END; $$;
DO $$ DECLARE n text; BEGIN
    FOREACH n IN ARRAY ARRAY['cost_revisions','cost_allocations','cost_revision_files','cost_links','cost_events'] LOOP
        EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION reject_immutable_change()','immutable_'||n,n);
        EXECUTE format('CREATE CONSTRAINT TRIGGER %I AFTER INSERT ON %I DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_cost()','validate_'||n,n);
    END LOOP;
END; $$;
CREATE CONSTRAINT TRIGGER validate_cost_head AFTER INSERT OR UPDATE ON actual_cost_entries DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_cost();
INSERT INTO roles(id,org_id,key,name,requires_mfa) SELECT gen_random_uuid(),o.id,r.key,r.name,r.mfa FROM organizations o CROSS JOIN (VALUES
    ('cost_recorder','费用登记人',false),('cost_confirmer','费用确认人',true),('cost_reverser','费用冲销人',true),('cost_reader','费用查阅人',false),('finance_reporter','财务报表查阅人',false)) r(key,name,mfa) ON CONFLICT(org_id,key) DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,a.action FROM roles CROSS JOIN (VALUES('cost.read')) a(action) WHERE key IN ('cost_recorder','cost_confirmer','cost_reverser','cost_reader','finance_reporter') ON CONFLICT DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,CASE key WHEN 'cost_recorder' THEN 'cost.record' WHEN 'cost_confirmer' THEN 'cost.confirm' WHEN 'cost_reverser' THEN 'cost.reverse' ELSE 'report.read' END FROM roles WHERE key IN ('cost_recorder','cost_confirmer','cost_reverser','finance_reporter') ON CONFLICT DO NOTHING;
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT,INSERT,UPDATE ON actual_cost_entries TO copypaste_app;
        GRANT SELECT,INSERT ON cost_revisions,cost_allocations,cost_revision_files,cost_links,cost_events TO copypaste_app;
        REVOKE UPDATE,DELETE ON cost_revisions,cost_allocations,cost_revision_files,cost_links,cost_events FROM copypaste_app;
        REVOKE DELETE ON actual_cost_entries FROM copypaste_app;
    END IF;
END; $$;
