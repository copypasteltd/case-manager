-- Published manual valuation books are immutable. Reports select an exact book;
-- corrections publish another revision, never rewrite old valuations/exports.
CREATE TABLE finance_fx_books (
 id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
 revision bigint NOT NULL CHECK(revision>0), creator_id uuid NOT NULL,
 title text NOT NULL CHECK(length(btrim(title)) BETWEEN 1 AND 200),
 base_currency text NOT NULL REFERENCES currency_definitions(code),
 rate_count integer NOT NULL CHECK(rate_count BETWEEN 0 AND 2000),
 content text NOT NULL CHECK(octet_length(content)<=2097152),
 content_sha256 text NOT NULL CHECK(content_sha256 ~ '^[a-f0-9]{64}$'),
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,id), UNIQUE(org_id,revision),
 FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id),
 CHECK(encode(sha256(convert_to(content,'UTF8')),'hex')=content_sha256)
);
CREATE INDEX finance_fx_books_history ON finance_fx_books(org_id,created_at DESC,id DESC);
CREATE TRIGGER finance_fx_book_immutable BEFORE UPDATE OR DELETE ON finance_fx_books
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION validate_finance_fx_book() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE body jsonb; item jsonb; previous_currency text; previous_end date;
BEGIN
 body:=NEW.content::jsonb;
 IF jsonb_typeof(body)<>'object' OR body->>'title' IS DISTINCT FROM NEW.title
  OR body->>'base_currency' IS DISTINCT FROM NEW.base_currency
  OR body->>'rounding' IS DISTINCT FROM 'scope_line_half_up'
  OR body->>'method' IS DISTINCT FROM 'manual_interval'
  OR jsonb_typeof(body->'rates') IS DISTINCT FROM 'array'
  OR jsonb_array_length(body->'rates')<>NEW.rate_count
  OR (SELECT count(*) FROM jsonb_object_keys(body))<>5
  OR NEW.revision<>(SELECT coalesce(max(revision),0)+1 FROM finance_fx_books WHERE org_id=NEW.org_id)
  OR NOT EXISTS(SELECT 1 FROM users WHERE org_id=NEW.org_id AND id=NEW.creator_id AND status='active') THEN
  RAISE EXCEPTION 'FX book requires exact attributed immutable content and next revision' USING ERRCODE='23514';
 END IF;
 FOR item IN SELECT value FROM jsonb_array_elements(body->'rates') LOOP
  IF jsonb_typeof(item)<>'object' OR (SELECT count(*) FROM jsonb_object_keys(item))<>6
   OR jsonb_typeof(item->'currency') IS DISTINCT FROM 'string'
   OR item->>'currency'=NEW.base_currency
   OR NOT EXISTS(SELECT 1 FROM currency_definitions WHERE code=item->>'currency')
   OR jsonb_typeof(item->'rate') IS DISTINCT FROM 'string'
   OR (item->>'rate' ~ '^(0|[1-9][0-9]{0,9})(\.[0-9]{1,10})?$') IS NOT TRUE
   OR (item->>'rate')::numeric<=0
   OR (item->>'valid_from' ~ '^\d{4}-\d{2}-\d{2}$') IS NOT TRUE
   OR (item->>'valid_to' ~ '^\d{4}-\d{2}-\d{2}$') IS NOT TRUE
   OR (item->>'rate_date' ~ '^\d{4}-\d{2}-\d{2}$') IS NOT TRUE
   OR (item->>'valid_to')::date<=(item->>'valid_from')::date
   OR (item->>'valid_to')::date-(item->>'valid_from')::date>366
   OR jsonb_typeof(item->'source') IS DISTINCT FROM 'string'
   OR length(btrim(item->>'source')) NOT BETWEEN 1 AND 200
   OR (previous_currency IS NOT NULL AND (item->>'currency'<previous_currency
     OR (item->>'currency'=previous_currency AND (item->>'valid_from')::date<previous_end))) THEN
   RAISE EXCEPTION 'FX quotes require ordered non-overlapping exact currencies, dates, sources and positive decimals' USING ERRCODE='23514';
  END IF;
  PERFORM (item->>'rate_date')::date;
  previous_currency:=item->>'currency'; previous_end:=(item->>'valid_to')::date;
 END LOOP;
 RETURN NEW;
END; $$;
CREATE TRIGGER finance_fx_book_binding BEFORE INSERT ON finance_fx_books
 FOR EACH ROW EXECUTE FUNCTION validate_finance_fx_book();
-- Company configuration follows the latest published book. Its historical
-- books retain their own base currencies when a later policy changes it.
CREATE FUNCTION guard_finance_base_currency() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.base_currency IS DISTINCT FROM OLD.base_currency AND
  NEW.base_currency IS DISTINCT FROM (SELECT base_currency FROM finance_fx_books WHERE org_id=NEW.id ORDER BY revision DESC LIMIT 1) THEN
  RAISE EXCEPTION 'base currency changes require the exact latest published FX book' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER finance_base_currency_policy BEFORE UPDATE OF base_currency ON organizations
 FOR EACH ROW EXECUTE FUNCTION guard_finance_base_currency();
INSERT INTO roles(id,org_id,key,name,requires_mfa)
 SELECT gen_random_uuid(),id,'finance_fx_manager','财务汇率管理人',true FROM organizations ON CONFLICT(org_id,key) DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action)
 SELECT org_id,id,a.action FROM roles CROSS JOIN (VALUES('report.read'),('finance.fx.manage')) a(action)
 WHERE key='finance_fx_manager' ON CONFLICT DO NOTHING;
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON finance_fx_books TO copypaste_app;
 REVOKE UPDATE,DELETE ON finance_fx_books FROM copypaste_app;
END IF; END; $$;
