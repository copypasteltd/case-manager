-- Keep old reminder/outbox rows immutable. Their original aggregate version
-- remains authoritative; nullable metadata is reserved for pre-upgrade rows.
ALTER TABLE subscription_reminders ADD COLUMN period_version bigint CHECK(period_version > 0);
ALTER TABLE subscription_reminders DROP CONSTRAINT subscription_reminders_org_id_period_id_days_before_key;
CREATE UNIQUE INDEX subscription_reminder_version_day ON subscription_reminders(org_id,period_id,period_version,days_before) WHERE period_version IS NOT NULL;
CREATE UNIQUE INDEX subscription_reminder_legacy_day ON subscription_reminders(org_id,period_id,days_before) WHERE period_version IS NULL;

CREATE OR REPLACE FUNCTION verify_subscription_reminder() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p subscription_periods;r subscription_period_revisions;j jobs;e outbox_events;
BEGIN
    SELECT * INTO STRICT p FROM subscription_periods WHERE org_id=NEW.org_id AND id=NEW.period_id FOR UPDATE;
    SELECT * INTO STRICT r FROM subscription_period_revisions WHERE org_id=p.org_id AND id=p.current_revision_id;
    SELECT * INTO j FROM jobs WHERE org_id=NEW.org_id AND id=NEW.job_id FOR UPDATE;
    SELECT * INTO e FROM outbox_events WHERE org_id=NEW.org_id AND id=NEW.outbox_id;
    IF NEW.period_version IS NULL OR NEW.period_version<>p.version
        OR j.id IS NULL OR j.kind<>'subscription.generate.v1' OR j.lane<>'finance' OR j.state<>'running' OR j.lease_until<=clock_timestamp() OR j.fencing_token<>NEW.job_fence OR j.lease_owner IS DISTINCT FROM NEW.job_owner OR j.payload->>'subscription_id' IS DISTINCT FROM p.subscription_id::text
        OR NEW.recipient_id<>r.owner_id OR NEW.remind_on<>p.start_date-NEW.days_before OR NEW.remind_on>(clock_timestamp() AT TIME ZONE r.timezone)::date OR NEW.remind_on<(clock_timestamp() AT TIME ZONE r.timezone)::date-366
        OR p.state<>'forecast' OR NOT subscription_charge_eligible(p.org_id,p.subscription_id,p.start_date)
        OR NOT EXISTS(SELECT 1 FROM subscription_revisions cfg WHERE cfg.org_id=p.org_id AND cfg.subscription_id=p.subscription_id AND cfg.id=r.configuration_revision_id AND cfg.payload->'reminder_days' @> jsonb_build_array(NEW.days_before))
        OR e.id IS NULL OR e.event_type<>'subscription.reminder.due' OR e.aggregate_type<>'subscription_period' OR e.aggregate_id<>p.id OR e.aggregate_version<>p.version
        OR e.payload->>'subscription_id' IS DISTINCT FROM p.subscription_id::text OR e.payload->>'period_version' IS DISTINCT FROM p.version::text
        OR e.payload->>'recipient_id' IS DISTINCT FROM NEW.recipient_id::text OR e.payload->>'period_id' IS DISTINCT FROM p.id::text OR e.payload->>'remind_on' IS DISTINCT FROM NEW.remind_on::text OR e.payload->>'days_before' IS DISTINCT FROM NEW.days_before::text OR e.payload->>'delivery_state' IS DISTINCT FROM 'queued'
        OR EXISTS(SELECT 1 FROM subscription_reminders n JOIN outbox_events prior_event ON prior_event.org_id=n.org_id AND prior_event.id=n.outbox_id WHERE n.org_id=NEW.org_id AND n.period_id=NEW.period_id AND n.days_before=NEW.days_before AND prior_event.aggregate_version=p.version) THEN
        RAISE EXCEPTION 'reminder must pin its period configuration version actual lease recipient date and queued outbox' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END; $$;
