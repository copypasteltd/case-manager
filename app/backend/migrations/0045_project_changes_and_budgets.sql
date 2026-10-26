-- Preserve old rows as protocol 0; do not manufacture an approval trail.
ALTER TABLE change_requests ADD COLUMN protocol_version integer NOT NULL DEFAULT 0 CHECK(protocol_version IN(0,1));
ALTER TABLE change_requests ADD COLUMN kind text NOT NULL DEFAULT 'change' CHECK(kind='change');
ALTER TABLE change_requests ADD COLUMN creator_id uuid;
ALTER TABLE change_requests ADD COLUMN classification text;
ALTER TABLE change_requests ADD COLUMN lifecycle text NOT NULL DEFAULT 'draft' CHECK(lifecycle IN('draft','requested','effective'));
ALTER TABLE change_requests ADD COLUMN approval_state text NOT NULL DEFAULT 'not_submitted' CHECK(approval_state IN('not_submitted','in_review','approved','returned','rejected','withdrawn'));
ALTER TABLE change_requests ADD COLUMN current_revision_id uuid;
ALTER TABLE change_requests ADD COLUMN approved_revision_id uuid;
ALTER TABLE change_requests ADD COLUMN request_id uuid;
ALTER TABLE change_requests ADD COLUMN deleted_at timestamptz CHECK(deleted_at IS NULL);
ALTER TABLE change_requests ADD FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id);
ALTER TABLE change_requests ADD FOREIGN KEY(org_id,request_id,kind,id) REFERENCES requests(org_id,id,kind,business_id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE change_requests ADD CHECK(protocol_version=0 OR (creator_id IS NOT NULL AND classification IN('internal','project_restricted','confidential') AND current_revision_id IS NOT NULL AND (approval_state='approved')=(approved_revision_id IS NOT NULL) AND (approved_revision_id IS NULL OR approved_revision_id=current_revision_id)));
CREATE INDEX change_requests_list ON change_requests(org_id,project_id,updated_at DESC,id DESC);
ALTER TABLE project_budgets ADD COLUMN protocol_version integer NOT NULL DEFAULT 0 CHECK(protocol_version IN(0,1));
ALTER TABLE project_budgets ADD COLUMN currency text REFERENCES currency_definitions(code);
ALTER TABLE project_budgets ADD COLUMN creator_id uuid;
ALTER TABLE project_budgets ADD COLUMN change_id uuid;
ALTER TABLE project_budgets ADD COLUMN change_revision_id uuid;
ALTER TABLE project_budgets ADD COLUMN predecessor_id uuid;
ALTER TABLE project_budgets ADD FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id);
ALTER TABLE project_budgets ADD FOREIGN KEY(org_id,change_id) REFERENCES change_requests(org_id,id);
ALTER TABLE project_budgets ADD FOREIGN KEY(org_id,predecessor_id) REFERENCES project_budgets(org_id,id);
ALTER TABLE project_budgets ADD CHECK(protocol_version=0 OR (currency IS NOT NULL AND creator_id IS NOT NULL AND change_id IS NOT NULL AND change_revision_id IS NOT NULL));
CREATE UNIQUE INDEX project_budget_active_currency ON project_budgets(org_id,project_id,currency) WHERE protocol_version=1 AND state='approved';
ALTER TABLE budget_lines ADD COLUMN ordinal integer;
ALTER TABLE budget_lines ADD COLUMN period_start date;
ALTER TABLE budget_lines ADD COLUMN period_end date;
ALTER TABLE budget_lines ADD UNIQUE(org_id,id);
ALTER TABLE budget_lines ADD UNIQUE(org_id,budget_id,ordinal);
ALTER TABLE budget_lines ADD CHECK(ordinal IS NULL OR (ordinal BETWEEN 1 AND 100 AND period_start IS NOT NULL AND period_end IS NOT NULL AND period_start<=period_end AND length(btrim(category)) BETWEEN 1 AND 200));
CREATE TABLE project_change_revisions (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,change_id uuid NOT NULL,seq bigint NOT NULL CHECK(seq>0),base_revision_id uuid,
 payload_schema_version integer NOT NULL DEFAULT 1 CHECK(payload_schema_version=1),payload jsonb NOT NULL,basis jsonb NOT NULL,
 content_hash text NOT NULL CHECK(content_hash~'^[0-9a-f]{64}$'),approval_amount numeric(20,6) NOT NULL CHECK(approval_amount>=0),budget_delta numeric(20,6) NOT NULL,
 target_budget_id uuid,author_id uuid NOT NULL,reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,id),UNIQUE(org_id,change_id,id),UNIQUE(org_id,change_id,seq),
 FOREIGN KEY(org_id,change_id) REFERENCES change_requests(org_id,id),FOREIGN KEY(org_id,author_id) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,change_id,base_revision_id) REFERENCES project_change_revisions(org_id,change_id,id),
 FOREIGN KEY(org_id,target_budget_id) REFERENCES project_budgets(org_id,id) DEFERRABLE INITIALLY DEFERRED,
 CHECK((seq=1)=(base_revision_id IS NULL)),CHECK(jsonb_typeof(payload)='object' AND jsonb_typeof(basis)='object')
);
ALTER TABLE change_requests ADD FOREIGN KEY(org_id,id,current_revision_id) REFERENCES project_change_revisions(org_id,change_id,id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE change_requests ADD FOREIGN KEY(org_id,id,approved_revision_id) REFERENCES project_change_revisions(org_id,change_id,id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE project_budgets ADD FOREIGN KEY(org_id,change_id,change_revision_id) REFERENCES project_change_revisions(org_id,change_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TRIGGER project_change_revision_immutable BEFORE UPDATE OR DELETE ON project_change_revisions FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER project_budget_lines_immutable BEFORE UPDATE OR DELETE ON budget_lines FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TABLE project_change_files (
 org_id uuid NOT NULL,revision_id uuid NOT NULL,version_id uuid NOT NULL,ordinal integer NOT NULL CHECK(ordinal BETWEEN 1 AND 20),
 PRIMARY KEY(org_id,revision_id,version_id),UNIQUE(org_id,revision_id,ordinal),
 FOREIGN KEY(org_id,revision_id) REFERENCES project_change_revisions(org_id,id),FOREIGN KEY(org_id,version_id) REFERENCES file_versions(org_id,id)
);
CREATE TRIGGER project_change_files_immutable BEFORE UPDATE OR DELETE ON project_change_files FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TABLE project_change_implementations (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,change_id uuid NOT NULL,revision_id uuid NOT NULL,request_revision_id uuid NOT NULL,
 project_id uuid NOT NULL,before_project_version bigint NOT NULL,after_project_version bigint NOT NULL,change_version bigint NOT NULL,
 actor_id uuid NOT NULL,session_id uuid NOT NULL,mfa_verified_at timestamptz NOT NULL,
 scope_verified boolean NOT NULL CHECK(scope_verified),budget_verified boolean NOT NULL CHECK(budget_verified),reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,id),UNIQUE(org_id,change_id),UNIQUE(org_id,project_id,after_project_version),
 FOREIGN KEY(org_id,change_id,revision_id) REFERENCES project_change_revisions(org_id,change_id,id),
 FOREIGN KEY(org_id,request_revision_id) REFERENCES request_revisions(org_id,id),FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id)
);
CREATE TRIGGER project_change_implementation_immutable BEFORE UPDATE OR DELETE ON project_change_implementations FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION project_budget_line_snapshot(o uuid,b uuid) RETURNS jsonb LANGUAGE sql STABLE AS $$
 SELECT COALESCE(jsonb_agg(jsonb_build_object('category',category,'period_start',period_start::text,'period_end',period_end::text,'amount',amount::text) ORDER BY ordinal),'[]'::jsonb) FROM budget_lines WHERE org_id=o AND budget_id=b
$$;
CREATE FUNCTION project_change_basis(o uuid,p uuid,c text) RETURNS jsonb LANGUAGE sql STABLE AS $$
 SELECT jsonb_build_object('project_id',x.id,'project_version',x.version,'name',x.name,'scope',x.scope,'start_date',x.start_date::text,'due_date',x.due_date::text,'classification',x.classification,'currency',c,
 'budget_id',b.id,'budget_lines',project_budget_line_snapshot(o,b.id),'budget_total',COALESCE((SELECT sum(amount) FROM budget_lines WHERE org_id=o AND budget_id=b.id),0)::numeric(20,6)::text)
 FROM projects x LEFT JOIN project_budgets b ON b.org_id=x.org_id AND b.project_id=x.id AND b.currency=c AND b.protocol_version=1 AND b.state='approved' WHERE x.org_id=o AND x.id=p
$$;
CREATE FUNCTION project_change_revision_hash(r project_change_revisions) RETURNS text LANGUAGE sql IMMUTABLE AS $$
 SELECT encode(sha256(convert_to(jsonb_build_object('schema',1,'change_id',r.change_id,'revision_id',r.id,'seq',r.seq,'content',r.payload,'basis',r.basis,'approval_amount',r.approval_amount::text,'budget_delta',r.budget_delta::text)::text,'UTF8')),'hex')
$$;
CREATE FUNCTION guard_project_change_head() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p projects; BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'change history cannot be deleted' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' AND (NEW.id,NEW.org_id,NEW.project_id,NEW.creator_id,NEW.classification,NEW.protocol_version,NEW.currency) IS DISTINCT FROM (OLD.id,OLD.org_id,OLD.project_id,OLD.creator_id,OLD.classification,OLD.protocol_version,OLD.currency) THEN RAISE EXCEPTION 'change identity is immutable' USING ERRCODE='23514'; END IF;
 SELECT * INTO p FROM projects WHERE org_id=NEW.org_id AND id=NEW.project_id FOR UPDATE;
 IF NEW.protocol_version<>1 OR p.id IS NULL OR p.state IN('closing','archived','cancelled') OR NEW.classification<>p.classification THEN RAISE EXCEPTION 'change project is read only or requires a separate evidence migration' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' AND (NEW.version<>OLD.version+1 OR (NEW.current_revision_id IS DISTINCT FROM OLD.current_revision_id AND OLD.approval_state IN('in_review','approved')) OR OLD.lifecycle='effective') THEN RAISE EXCEPTION 'change requires exact next draft or workflow transition' USING ERRCODE='23514'; END IF;
 IF TG_OP='INSERT' AND (NEW.version<>1 OR NEW.lifecycle<>'draft' OR NEW.approval_state<>'not_submitted' OR NEW.request_id IS NOT NULL) THEN RAISE EXCEPTION 'change starts as a real draft' USING ERRCODE='23514'; END IF;
 NEW.state:=CASE WHEN NEW.lifecycle='effective' THEN 'implemented' WHEN NEW.approval_state IN('approved','in_review','rejected','withdrawn') THEN NEW.approval_state ELSE 'draft' END;
 RETURN NEW;
END; $$;
CREATE TRIGGER project_change_head_guard BEFORE INSERT OR UPDATE OR DELETE ON change_requests FOR EACH ROW EXECUTE FUNCTION guard_project_change_head();
CREATE FUNCTION prepare_project_change_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE d change_requests;p projects;line jsonb;currency_exp integer;new_total numeric:=0;increase numeric:=0;old_amount numeric;rid uuid;
BEGIN
 IF NOT (NEW.payload ?& ARRAY['title','category','currency','scope','scope_delta','schedule_delta','start_date','due_date','budget_lines','file_version_ids'])
 OR EXISTS(SELECT 1 FROM jsonb_each(NEW.payload) x WHERE x.key NOT IN('title','category','currency','scope','scope_delta','schedule_delta','start_date','due_date','budget_lines','file_version_ids'))
 OR EXISTS(SELECT 1 FROM unnest(ARRAY['title','category','currency','scope','scope_delta','schedule_delta']) k WHERE jsonb_typeof(NEW.payload->k) IS DISTINCT FROM 'string')
 OR (jsonb_typeof(NEW.payload->'start_date') NOT IN('string','null')) OR (jsonb_typeof(NEW.payload->'due_date') NOT IN('string','null'))
 OR jsonb_typeof(NEW.payload->'budget_lines') NOT IN('array','null') OR jsonb_typeof(NEW.payload->'file_version_ids') IS DISTINCT FROM 'array'
 THEN RAISE EXCEPTION 'change proposal requires its exact schema' USING ERRCODE='23514'; END IF;
 SELECT * INTO d FROM change_requests WHERE org_id=NEW.org_id AND id=NEW.change_id;
 SELECT * INTO p FROM projects WHERE org_id=d.org_id AND id=d.project_id FOR UPDATE;
 SELECT * INTO d FROM change_requests WHERE org_id=NEW.org_id AND id=NEW.change_id FOR UPDATE;
 IF d.id IS NULL OR d.protocol_version<>1 OR d.approval_state IN('in_review','approved') OR d.lifecycle<>'draft' OR p.state IN('closing','archived','cancelled')
 OR NEW.seq<>(SELECT COALESCE(max(seq),0)+1 FROM project_change_revisions WHERE org_id=NEW.org_id AND change_id=NEW.change_id)
 OR (NEW.seq>1 AND NEW.base_revision_id IS DISTINCT FROM d.current_revision_id)
 OR (NEW.seq>1 AND NEW.payload->>'currency' IS DISTINCT FROM d.currency)
 OR NEW.basis IS DISTINCT FROM project_change_basis(d.org_id,p.id,NEW.payload->>'currency')
 OR length(btrim(NEW.payload->>'title')) NOT BETWEEN 1 AND 200 OR length(btrim(NEW.payload->>'category')) NOT BETWEEN 1 AND 200
 OR length(btrim(NEW.payload->>'scope')) NOT BETWEEN 1 AND 20000 OR length(btrim(NEW.payload->>'scope_delta')) NOT BETWEEN 1 AND 20000
 OR length(btrim(NEW.payload->>'schedule_delta')) NOT BETWEEN 1 AND 20000
 OR jsonb_typeof(NEW.payload->'file_version_ids')<>'array' OR jsonb_array_length(NEW.payload->'file_version_ids')>20
 OR (NEW.payload->>'start_date' IS NOT NULL AND (NEW.payload->>'start_date' !~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' OR (NEW.payload->>'start_date')::date::text IS DISTINCT FROM NEW.payload->>'start_date'))
 OR (NEW.payload->>'due_date' IS NOT NULL AND (NEW.payload->>'due_date' !~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' OR (NEW.payload->>'due_date')::date::text IS DISTINCT FROM NEW.payload->>'due_date'))
 OR ((NEW.payload->>'start_date')::date>(NEW.payload->>'due_date')::date) THEN RAISE EXCEPTION 'change revision requires complete exact project baseline and proposal' USING ERRCODE='23514'; END IF;
 SELECT c.exponent INTO currency_exp FROM currency_definitions c WHERE code=NEW.payload->>'currency';
 IF currency_exp IS NULL THEN RAISE EXCEPTION 'unsupported change currency' USING ERRCODE='23514'; END IF;
 IF NEW.payload->'budget_lines'<>'null'::jsonb THEN
  IF jsonb_typeof(NEW.payload->'budget_lines')<>'array' OR jsonb_array_length(NEW.payload->'budget_lines')>100 THEN RAISE EXCEPTION 'invalid budget collection' USING ERRCODE='23514'; END IF;
  FOR line IN SELECT value FROM jsonb_array_elements(NEW.payload->'budget_lines') LOOP
   IF NOT (line ?& ARRAY['category','period_start','period_end','amount']) OR (SELECT count(*) FROM jsonb_object_keys(line))<>4 OR EXISTS(SELECT 1 FROM jsonb_each(line) x WHERE jsonb_typeof(x.value) IS DISTINCT FROM 'string') OR length(btrim(line->>'category')) NOT BETWEEN 1 AND 200 OR line->>'period_start' !~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' OR line->>'period_end' !~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' OR (line->>'period_start')::date IS NULL OR (line->>'period_end')::date IS NULL OR (line->>'period_start')::date>(line->>'period_end')::date OR line->>'amount' !~ '^(0|[1-9][0-9]{0,13})(\.[0-9]{1,6})?$' OR (line->>'amount')::numeric<>round((line->>'amount')::numeric,currency_exp) THEN RAISE EXCEPTION 'invalid dated original currency budget line' USING ERRCODE='23514'; END IF;
   SELECT COALESCE(sum((x->>'amount')::numeric),0) INTO old_amount FROM jsonb_array_elements(NEW.basis->'budget_lines')x WHERE (x->>'category',x->>'period_start',x->>'period_end')=(line->>'category',line->>'period_start',line->>'period_end');
   new_total:=new_total+(line->>'amount')::numeric;increase:=increase+greatest((line->>'amount')::numeric-old_amount,0);
  END LOOP;
  IF EXISTS(SELECT 1 FROM jsonb_array_elements(NEW.payload->'budget_lines')WITH ORDINALITY a(v,n) JOIN jsonb_array_elements(NEW.payload->'budget_lines')WITH ORDINALITY b(v,n) ON a.n<b.n AND a.v->>'category'=b.v->>'category' AND (a.v->>'period_start')::date<=(b.v->>'period_end')::date AND (b.v->>'period_start')::date<=(a.v->>'period_end')::date) THEN RAISE EXCEPTION 'budget category periods overlap' USING ERRCODE='23514'; END IF;
  NEW.approval_amount:=increase;NEW.budget_delta:=new_total-(NEW.basis->>'budget_total')::numeric;NEW.target_budget_id:=gen_random_uuid();
 ELSE NEW.approval_amount:=0;NEW.budget_delta:=0;NEW.target_budget_id:=NULL; END IF;
 NEW.content_hash:=project_change_revision_hash(NEW);RETURN NEW;
END; $$;
CREATE TRIGGER project_change_revision_guard BEFORE INSERT ON project_change_revisions FOR EACH ROW EXECUTE FUNCTION prepare_project_change_revision();
CREATE FUNCTION create_project_change_budget() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE d change_requests;BEGIN
 IF NEW.target_budget_id IS NULL THEN RETURN NULL; END IF;
 SELECT * INTO d FROM change_requests WHERE org_id=NEW.org_id AND id=NEW.change_id;
 INSERT INTO project_budgets(id,org_id,project_id,revision,state,protocol_version,currency,creator_id,change_id,change_revision_id,predecessor_id)
 SELECT NEW.target_budget_id,NEW.org_id,d.project_id,COALESCE(max(revision),0)+1,'draft',1,NEW.payload->>'currency',NEW.author_id,d.id,NEW.id,(NEW.basis->>'budget_id')::uuid FROM project_budgets WHERE org_id=NEW.org_id AND project_id=d.project_id;
 INSERT INTO budget_lines(id,org_id,budget_id,category,currency,amount,ordinal,period_start,period_end)
 SELECT gen_random_uuid(),NEW.org_id,NEW.target_budget_id,v->>'category',NEW.payload->>'currency',(v->>'amount')::numeric,n,(v->>'period_start')::date,(v->>'period_end')::date FROM jsonb_array_elements(NEW.payload->'budget_lines')WITH ORDINALITY x(v,n);
 RETURN NULL;
END; $$;
CREATE TRIGGER project_change_budget_draft AFTER INSERT ON project_change_revisions FOR EACH ROW EXECUTE FUNCTION create_project_change_budget();
ALTER TABLE file_version_references DROP CONSTRAINT file_version_references_owner_type_check;
ALTER TABLE file_version_references ADD CHECK(owner_type IN('knowledge_revision','comment','approval_snapshot','contract_revision','contract_signature','purchase_revision','purchase_order','purchase_receipt','claim_revision','cost_revision','payment_revision','subscription_revision','subscription_period_event','contract_finance_event','contract_closure','purchase_variance_revision','purchase_completion','deliverable','project_change_revision','recovery_set'));
CREATE FUNCTION pin_project_change_file() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id) SELECT gen_random_uuid(),NEW.org_id,file_id,id,'project_change_revision',NEW.revision_id FROM file_versions WHERE org_id=NEW.org_id AND id=NEW.version_id;RETURN NEW;
END; $$;
CREATE TRIGGER project_change_file_reference AFTER INSERT ON project_change_files FOR EACH ROW EXECUTE FUNCTION pin_project_change_file();
CREATE FUNCTION validate_project_change() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE did uuid;d change_requests;r project_change_revisions;q requests;files jsonb; BEGIN
 IF TG_TABLE_NAME='requests' THEN IF NEW.kind<>'change' THEN RETURN NULL; END IF;did:=NEW.business_id;
 ELSIF TG_TABLE_NAME='change_requests' THEN did:=NEW.id;
 ELSIF TG_TABLE_NAME='project_change_revisions' THEN did:=NEW.change_id;
 ELSE SELECT change_id INTO did FROM project_change_revisions WHERE org_id=NEW.org_id AND id=NEW.revision_id; END IF;
 SELECT * INTO d FROM change_requests WHERE org_id=NEW.org_id AND id=did;
 IF d.id IS NULL THEN RAISE EXCEPTION 'change approval needs exact business' USING ERRCODE='23514'; END IF;
 IF d.protocol_version=0 THEN RETURN NULL; END IF;
 SELECT * INTO r FROM project_change_revisions WHERE org_id=d.org_id AND change_id=d.id AND id=d.current_revision_id;
 IF r.id IS NULL OR r.content_hash<>project_change_revision_hash(r) OR r.seq<>(SELECT max(seq) FROM project_change_revisions WHERE org_id=d.org_id AND change_id=d.id)
 OR (d.title,d.scope_delta,d.base_project_version,d.currency,d.budget_delta) IS DISTINCT FROM (r.payload->>'title',r.payload->>'scope_delta',(r.basis->>'project_version')::bigint,r.payload->>'currency',r.budget_delta)
 OR ((d.lifecycle='effective')<>EXISTS(SELECT 1 FROM project_change_implementations WHERE org_id=d.org_id AND change_id=d.id)) THEN RAISE EXCEPTION 'change head requires exact immutable current revision and implementation' USING ERRCODE='23514'; END IF;
 IF d.request_id IS NULL THEN IF d.approval_state<>'not_submitted' THEN RAISE EXCEPTION 'change needs request' USING ERRCODE='23514'; END IF;
 ELSE SELECT * INTO q FROM requests WHERE org_id=d.org_id AND id=d.request_id;
 IF q.id IS NULL OR q.kind<>'change' OR q.business_id<>d.id OR (q.project_id,q.classification) IS DISTINCT FROM (d.project_id,d.classification) OR d.approval_state IS DISTINCT FROM (CASE WHEN q.state='draft' THEN 'not_submitted' ELSE q.state END) THEN RAISE EXCEPTION 'change request state or scope differs' USING ERRCODE='23514'; END IF;END IF;
 IF (d.approval_state IN('in_review','approved')) IS DISTINCT FROM (d.lifecycle IN('requested','effective')) THEN RAISE EXCEPTION 'change workflow lifecycle differs' USING ERRCODE='23514'; END IF;
 -- Every newly inserted revision must have exactly its complete ordered file set.
 IF TG_TABLE_NAME='project_change_revisions' THEN SELECT * INTO r FROM project_change_revisions WHERE org_id=NEW.org_id AND id=NEW.id;
 ELSIF TG_TABLE_NAME='project_change_files' THEN SELECT * INTO r FROM project_change_revisions WHERE org_id=NEW.org_id AND id=NEW.revision_id;END IF;
 SELECT COALESCE(jsonb_agg(version_id::text ORDER BY ordinal),'[]'::jsonb) INTO files FROM project_change_files WHERE org_id=r.org_id AND revision_id=r.id;
 IF files IS DISTINCT FROM r.payload->'file_version_ids' OR (r.target_budget_id IS NOT NULL AND project_budget_line_snapshot(r.org_id,r.target_budget_id) IS DISTINCT FROM r.payload->'budget_lines') THEN RAISE EXCEPTION 'change needs complete fixed file and budget lines' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER project_change_head_consistency AFTER INSERT OR UPDATE ON change_requests DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_project_change();
CREATE CONSTRAINT TRIGGER project_change_revision_consistency AFTER INSERT ON project_change_revisions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_project_change();
CREATE CONSTRAINT TRIGGER project_change_files_consistency AFTER INSERT ON project_change_files DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_project_change();
CREATE CONSTRAINT TRIGGER project_change_request_consistency AFTER INSERT OR UPDATE ON requests DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_project_change();
CREATE FUNCTION validate_project_change_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r project_change_revisions;d change_requests; BEGIN
 IF NOT EXISTS(SELECT 1 FROM requests WHERE org_id=NEW.org_id AND id=NEW.request_id AND kind='change') THEN RETURN NEW; END IF;
 SELECT v.* INTO r FROM project_change_revisions v JOIN requests q ON q.org_id=v.org_id AND q.business_id=v.change_id WHERE q.org_id=NEW.org_id AND q.id=NEW.request_id AND v.id=NEW.business_revision_id;
 SELECT * INTO d FROM change_requests WHERE org_id=r.org_id AND id=r.change_id;
 IF r.id IS NULL OR NEW.business_content_hash<>r.content_hash OR NEW.payload->>'kind' IS DISTINCT FROM 'change' OR NEW.payload->>'business_id' IS DISTINCT FROM d.id::text OR NEW.payload->>'business_revision_id' IS DISTINCT FROM r.id::text OR NEW.payload->>'business_content_hash' IS DISTINCT FROM r.content_hash OR NEW.payload->'content' IS DISTINCT FROM r.payload OR NEW.payload->'project_change_basis' IS DISTINCT FROM r.basis OR (NEW.payload->>'project_change_budget_delta')::numeric IS DISTINCT FROM r.budget_delta OR (NEW.payload->>'amount')::numeric IS DISTINCT FROM r.approval_amount OR (NEW.payload->>'project_id')::uuid IS DISTINCT FROM d.project_id OR NEW.payload->>'classification' IS DISTINCT FROM d.classification OR (SELECT COALESCE(jsonb_agg(x->>'version_id' ORDER BY n),'[]'::jsonb) FROM jsonb_array_elements(NEW.payload->'attachments')WITH ORDINALITY a(x,n)) IS DISTINCT FROM r.payload->'file_version_ids' THEN RAISE EXCEPTION 'change approval must pin exact proposal and baseline' USING ERRCODE='23514'; END IF;RETURN NEW;
END; $$;
CREATE TRIGGER project_change_snapshot_guard BEFORE INSERT ON request_revisions FOR EACH ROW EXECUTE FUNCTION validate_project_change_snapshot();
CREATE FUNCTION guard_project_budget() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r project_change_revisions;d change_requests; BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'budget history cannot be deleted' USING ERRCODE='23514'; END IF;
 IF NEW.protocol_version<>1 THEN RAISE EXCEPTION 'legacy budget needs evidence migration' USING ERRCODE='23514'; END IF;
 SELECT * INTO r FROM project_change_revisions WHERE org_id=NEW.org_id AND id=NEW.change_revision_id;
 SELECT * INTO d FROM change_requests WHERE org_id=NEW.org_id AND id=NEW.change_id;
 IF r.id IS NULL OR r.change_id<>d.id OR NEW.project_id<>d.project_id OR NEW.currency<>r.payload->>'currency' OR NEW.id IS DISTINCT FROM r.target_budget_id THEN RAISE EXCEPTION 'budget needs its fixed change revision' USING ERRCODE='23514'; END IF;
 IF TG_OP='INSERT' THEN IF NEW.state<>'draft' OR NEW.creator_id<>r.author_id OR NEW.predecessor_id IS DISTINCT FROM (r.basis->>'budget_id')::uuid THEN RAISE EXCEPTION 'budget starts as a fixed draft' USING ERRCODE='23514'; END IF;
 ELSE
 IF (to_jsonb(NEW)-'state') IS DISTINCT FROM (to_jsonb(OLD)-'state') OR NOT ((OLD.state='draft' AND NEW.state='approved' AND EXISTS(SELECT 1 FROM project_change_implementations WHERE org_id=NEW.org_id AND revision_id=r.id)) OR (OLD.state='approved' AND NEW.state='superseded' AND EXISTS(SELECT 1 FROM project_change_implementations i JOIN project_change_revisions v ON v.org_id=i.org_id AND v.id=i.revision_id WHERE i.org_id=NEW.org_id AND i.project_id=NEW.project_id AND (v.basis->>'budget_id')::uuid=NEW.id))) THEN RAISE EXCEPTION 'budget only advances by an exact approved implementation' USING ERRCODE='23514'; END IF;
 END IF;RETURN NEW;
END; $$;
CREATE TRIGGER project_budget_guard BEFORE INSERT OR UPDATE OR DELETE ON project_budgets FOR EACH ROW EXECUTE FUNCTION guard_project_budget();
CREATE FUNCTION guard_project_budget_line() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE b project_budgets;r project_change_revisions; BEGIN
 SELECT * INTO b FROM project_budgets WHERE org_id=NEW.org_id AND id=NEW.budget_id;
 SELECT * INTO r FROM project_change_revisions WHERE org_id=b.org_id AND id=b.change_revision_id;
 IF b.id IS NULL OR r.id IS NULL OR NEW.ordinal IS NULL OR b.protocol_version<>1 OR b.state<>'draft' OR NEW.currency<>b.currency OR jsonb_build_object('category',NEW.category,'period_start',NEW.period_start::text,'period_end',NEW.period_end::text,'amount',NEW.amount::text) IS DISTINCT FROM r.payload->'budget_lines'->(NEW.ordinal-1) THEN RAISE EXCEPTION 'budget line must equal its immutable change proposal' USING ERRCODE='23514'; END IF;RETURN NEW;
END; $$;
CREATE TRIGGER project_budget_line_guard BEFORE INSERT ON budget_lines FOR EACH ROW EXECUTE FUNCTION guard_project_budget_line();
CREATE FUNCTION prepare_project_change_implementation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p projects;d change_requests;r project_change_revisions;q requests;s sessions;ids uuid[]; BEGIN
 SELECT * INTO p FROM projects WHERE org_id=NEW.org_id AND id=NEW.project_id FOR UPDATE;
 SELECT * INTO d FROM change_requests WHERE org_id=NEW.org_id AND id=NEW.change_id FOR UPDATE;
 SELECT * INTO r FROM project_change_revisions WHERE org_id=d.org_id AND change_id=d.id AND id=NEW.revision_id;
 SELECT * INTO q FROM requests WHERE org_id=d.org_id AND id=d.request_id FOR UPDATE;
 SELECT * INTO s FROM sessions WHERE org_id=NEW.org_id AND id=NEW.session_id FOR SHARE;
 SELECT array_agg(version_id ORDER BY ordinal) INTO ids FROM project_change_files WHERE org_id=r.org_id AND revision_id=r.id;
 PERFORM f.id FROM files f WHERE f.org_id=NEW.org_id AND f.id IN(SELECT file_id FROM file_versions WHERE org_id=NEW.org_id AND id=ANY(ids)) ORDER BY f.id FOR SHARE;
 IF p.id IS NULL OR d.id IS NULL OR d.protocol_version<>1 OR d.project_id<>p.id OR p.state IN('closing','archived','cancelled') OR d.lifecycle<>'requested' OR d.approval_state<>'approved' OR r.id IS NULL OR d.current_revision_id IS DISTINCT FROM r.id OR d.approved_revision_id IS DISTINCT FROM r.id
 OR q.id IS NULL OR q.state<>'approved' OR q.current_revision_id IS DISTINCT FROM NEW.request_revision_id OR q.business_id<>d.id OR q.kind<>'change'
 OR NOT EXISTS(SELECT 1 FROM request_revisions WHERE org_id=q.org_id AND id=q.current_revision_id AND business_revision_id=r.id AND business_content_hash=r.content_hash)
 OR r.basis IS DISTINCT FROM project_change_basis(p.org_id,p.id,r.payload->>'currency') OR NEW.before_project_version<>p.version OR NEW.after_project_version<>p.version+1 OR NEW.change_version<>d.version+1
 OR s.id IS NULL OR s.device_type NOT IN('web','android','wechat','douyin') OR s.user_id<>NEW.actor_id OR s.revoked_at IS NOT NULL OR s.expires_at<=clock_timestamp() OR s.mfa_verified_at IS NULL OR s.mfa_verified_at IS DISTINCT FROM NEW.mfa_verified_at OR s.mfa_verified_at<clock_timestamp()-interval '10 minutes' OR s.mfa_verified_at>clock_timestamp()
 OR NOT EXISTS(SELECT 1 FROM users WHERE org_id=s.org_id AND id=s.user_id AND status='active' AND auth_version=s.auth_version)
 OR COALESCE(cardinality(ids),0)<1 OR NOT deliverable_files_available(p.org_id,p.id,p.classification,ids) THEN RAISE EXCEPTION 'implementation needs exact approved proposal baseline files and actual human MFA' USING ERRCODE='23514'; END IF;
 NEW.created_at:=clock_timestamp();RETURN NEW;
END; $$;
CREATE TRIGGER project_change_implementation_guard BEFORE INSERT ON project_change_implementations FOR EACH ROW EXECUTE FUNCTION prepare_project_change_implementation();
CREATE FUNCTION apply_project_change_implementation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r project_change_revisions; BEGIN
 SELECT * INTO r FROM project_change_revisions WHERE org_id=NEW.org_id AND id=NEW.revision_id;
 IF r.target_budget_id IS NOT NULL THEN
 UPDATE project_budgets SET state='superseded' WHERE org_id=NEW.org_id AND id=(r.basis->>'budget_id')::uuid;
 UPDATE project_budgets SET state='approved' WHERE org_id=NEW.org_id AND id=r.target_budget_id;
 END IF;
 UPDATE projects SET scope=r.payload->>'scope',start_date=(r.payload->>'start_date')::date,due_date=(r.payload->>'due_date')::date,version=NEW.after_project_version,updated_at=NEW.created_at WHERE org_id=NEW.org_id AND id=NEW.project_id;
 UPDATE change_requests SET lifecycle='effective',version=NEW.change_version,updated_at=NEW.created_at WHERE org_id=NEW.org_id AND id=NEW.change_id;
 RETURN NULL;
END; $$;
CREATE TRIGGER project_change_apply AFTER INSERT ON project_change_implementations FOR EACH ROW EXECUTE FUNCTION apply_project_change_implementation();
CREATE FUNCTION guard_project_controlled_scope() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF OLD.state IN('active','on_hold') AND (NEW.scope,NEW.start_date,NEW.due_date) IS DISTINCT FROM (OLD.scope,OLD.start_date,OLD.due_date) AND NOT EXISTS(SELECT 1 FROM project_change_implementations i JOIN project_change_revisions r ON r.org_id=i.org_id AND r.id=i.revision_id WHERE i.org_id=NEW.org_id AND i.project_id=NEW.id AND i.before_project_version=OLD.version AND i.after_project_version=NEW.version AND (r.payload->>'scope',(r.payload->>'start_date')::date,(r.payload->>'due_date')::date) IS NOT DISTINCT FROM (NEW.scope,NEW.start_date,NEW.due_date)) THEN RAISE EXCEPTION 'active project scope and dates require an approved change implementation' USING ERRCODE='23514'; END IF;RETURN NEW;
END; $$;
CREATE TRIGGER project_controlled_scope BEFORE UPDATE ON projects FOR EACH ROW EXECUTE FUNCTION guard_project_controlled_scope();
INSERT INTO roles(id,org_id,key,name,requires_mfa) SELECT gen_random_uuid(),o.id,r.key,r.name,r.mfa FROM organizations o CROSS JOIN (VALUES('project_change_reader','项目变更与预算查阅人',false),('project_change_implementer','项目变更实施人',true))r(key,name,mfa) ON CONFLICT(org_id,key) DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action) SELECT r.org_id,r.id,a.action FROM roles r CROSS JOIN (VALUES('change.read'),('budget.read'))a(action) WHERE r.key IN('project_manager','project_change_reader','project_change_implementer') ON CONFLICT DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,'change.update' FROM roles WHERE key='project_manager' ON CONFLICT DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,a.action FROM roles CROSS JOIN (VALUES('project.read'),('project.update'),('change.implement'))a(action) WHERE key='project_change_implementer' ON CONFLICT DO NOTHING;
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON project_change_revisions,project_change_files,project_change_implementations TO copypaste_app;
 REVOKE UPDATE,DELETE ON project_change_revisions,project_change_files,project_change_implementations,budget_lines FROM copypaste_app;
 REVOKE DELETE ON change_requests,project_budgets FROM copypaste_app;
END IF;END; $$;
