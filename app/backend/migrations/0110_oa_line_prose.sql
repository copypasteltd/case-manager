-- Extend exact human-selected prose to existing purchase/claim line descriptions.
-- No historical intent, suggestion, adoption or commercial fact is rewritten.
CREATE FUNCTION agent_oa_text_field_limit(kind text, field text, content jsonb)
RETURNS integer LANGUAGE plpgsql IMMUTABLE STRICT AS $$
DECLARE line_id text; matches integer;
BEGIN
 IF kind='contract' AND field=ANY(ARRAY['renewal_terms','termination_terms','notes']) THEN RETURN 20000; END IF;
 IF kind IN ('purchase','claim') THEN
  IF field='notes' THEN RETURN 20000; END IF;
  IF field='purpose' THEN RETURN 4000; END IF;
  IF field ~ '^line:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}:description$' THEN
   line_id := substring(field FROM 6 FOR 36);
   SELECT count(*) INTO matches FROM jsonb_array_elements(coalesce(content->'lines','[]'::jsonb)) l WHERE l->>'id'=line_id;
   IF matches=1 THEN RETURN 1000; END IF;
  END IF;
 END IF;
 RETURN 0;
END; $$;

CREATE FUNCTION validate_agent_oa_text_selection() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE field text; wanted jsonb; unique_count integer;
BEGIN
 IF NEW.tool<>'draft_business_text' THEN RETURN NEW; END IF;
 wanted := NEW.arguments->'fields';
 IF jsonb_typeof(wanted) IS DISTINCT FROM 'array' THEN
  RAISE EXCEPTION 'text selection must be a finite array' USING ERRCODE='23514';
 END IF;
 SELECT count(DISTINCT v) INTO unique_count FROM jsonb_array_elements_text(wanted) f(v);
 IF jsonb_array_length(wanted) NOT BETWEEN 1 AND 102 OR unique_count<>jsonb_array_length(wanted) THEN
  RAISE EXCEPTION 'text selection must be nonempty and unique' USING ERRCODE='23514';
 END IF;
 FOR field IN SELECT jsonb_array_elements_text(wanted) LOOP
  IF coalesce(agent_oa_text_field_limit(NEW.kind,field,NEW.before_data#>'{business,payload,content}'),0)=0 THEN
   RAISE EXCEPTION 'text field must belong to the exact original business content' USING ERRCODE='23514';
  END IF;
 END LOOP;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_oa_text_selection_binding BEFORE INSERT ON agent_oa_intents FOR EACH ROW EXECUTE FUNCTION validate_agent_oa_text_selection();

CREATE OR REPLACE FUNCTION validate_agent_oa_text() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE i agent_oa_intents; x agent_runs; wanted jsonb; actual jsonb; pair record;
BEGIN
 SELECT * INTO i FROM agent_oa_intents WHERE org_id=NEW.org_id AND run_id=NEW.run_id;
 SELECT * INTO x FROM agent_runs WHERE org_id=NEW.org_id AND id=NEW.run_id;
 IF i.tool IS DISTINCT FROM 'draft_business_text' OR x.actor_id IS DISTINCT FROM i.actor_id THEN
  RAISE EXCEPTION 'text proposal requires its exact delegated intent' USING ERRCODE='23514';
 END IF;
 IF TG_TABLE_NAME='agent_oa_text_drafts' THEN
  IF x.state<>'running' OR x.cancel_requested_at IS NOT NULL OR x.expires_at<=clock_timestamp() THEN
   RAISE EXCEPTION 'text proposal requires an active run' USING ERRCODE='23514';
  END IF;
  SELECT jsonb_agg(v ORDER BY v) INTO wanted FROM jsonb_array_elements_text(i.arguments->'fields') f(v);
  SELECT jsonb_agg(v ORDER BY v) INTO actual FROM jsonb_object_keys(NEW.fields::jsonb) f(v);
  IF wanted IS NULL OR actual IS DISTINCT FROM wanted OR jsonb_array_length(wanted) NOT BETWEEN 1 AND 102 THEN
   RAISE EXCEPTION 'text fields must match exact human selection' USING ERRCODE='23514';
  END IF;
  FOR pair IN SELECT key,value FROM jsonb_each(NEW.fields::jsonb) LOOP
   IF coalesce(agent_oa_text_field_limit(i.kind,pair.key,i.before_data#>'{business,payload,content}'),0)=0
    OR jsonb_typeof(pair.value)<>'string' OR length(btrim(pair.value#>>'{}'))=0
    OR length(pair.value#>>'{}')>agent_oa_text_field_limit(i.kind,pair.key,i.before_data#>'{business,payload,content}') THEN
    RAISE EXCEPTION 'invalid business prose field' USING ERRCODE='23514';
   END IF;
  END LOOP;
 ELSE
  IF x.state<>'succeeded' OR x.cancel_requested_at IS NOT NULL OR NOT EXISTS(
   SELECT 1 FROM idempotency_records o WHERE o.org_id=i.org_id AND o.id=NEW.operation_id AND o.actor_id=i.actor_id
    AND o.route_key='/internal/agent-oa-text-adopt/'||i.kind AND o.idempotency_key='agent-oa-text:'||i.run_id::text
    AND o.state='succeeded' AND o.response_data->>'id'=i.business_id::text
    AND o.response_data->>'current_revision_id'=NEW.result_data->>'current_revision_id'
    AND o.response_data->>'content_hash'=NEW.result_data->>'content_hash'
    AND (o.response_data->>'version')::bigint=(i.before_data->'business'->>'version')::bigint+1
    AND o.response_data->'version'=NEW.result_data->'version')
   OR (NEW.result_data->>'id'=i.business_id::text AND NEW.result_data->>'kind'=i.kind) IS NOT TRUE THEN
   RAISE EXCEPTION 'adoption requires exact normal human business receipt' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END; $$;
