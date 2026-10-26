-- A fully delivered old-price order may leave unused value in a newer plan.
-- Preserve existing completion snapshots and require a disposition on new reviews.
ALTER FUNCTION purchase_completion_has_difference(jsonb) RENAME TO purchase_completion_has_difference_0042_base;
CREATE FUNCTION purchase_completion_has_difference(b jsonb) RETURNS boolean LANGUAGE sql IMMUTABLE AS $$
    SELECT purchase_completion_has_difference_0042_base(b) OR
        (b->>'execution_amount')::numeric IS DISTINCT FROM
        COALESCE((SELECT sum((l->>'reserved_amount')::numeric) FROM jsonb_array_elements(b->'lines')l),0)
$$;
