ALTER TABLE jobs ADD CONSTRAINT job_attempt_limits CHECK(attempt BETWEEN 0 AND max_attempts AND max_attempts BETWEEN 1 AND 16);
ALTER TABLE jobs ADD CONSTRAINT job_fence_positive CHECK(fencing_token>=0);
ALTER TABLE jobs ADD CONSTRAINT job_lease_state CHECK(
    (state='running' AND lease_until IS NOT NULL AND lease_owner IS NOT NULL)
    OR (state<>'running' AND lease_until IS NULL AND lease_owner IS NULL));
CREATE TRIGGER job_input_immutable BEFORE UPDATE OF org_id,kind,lane,dedup_key,payload_version,payload,max_attempts
ON jobs FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
