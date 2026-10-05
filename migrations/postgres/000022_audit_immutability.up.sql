-- Audit immutability at the database level: the application role can no
-- longer UPDATE or DELETE audit rows, closing the gap between the
-- "immutable trail" contract and what SQL alone could do. Retention stays
-- possible through the parameterized SECURITY DEFINER prune function, and
-- the demo seeder keeps its reset path (demo-% rows).

CREATE FUNCTION audit_block_mutation() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'audit_log is append-only (attempted %)', TG_OP;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER audit_log_no_mutation
  BEFORE UPDATE OR DELETE ON audit_log
  FOR EACH ROW
  WHEN (current_setting('app.audit_pruning', true) IS DISTINCT FROM '1'
        AND OLD.actor_id NOT LIKE 'demo-%')
  EXECUTE FUNCTION audit_block_mutation();

-- The only sanctioned delete path: parameterized by age alone, defined by
-- the table owner, callable by the application role for retention sweeps.
-- The transaction-local setting marks the trigger-suppressed window, so the
-- blocking trigger stays armed for every other statement in the session.
CREATE FUNCTION audit_prune(older_than timestamptz) RETURNS bigint AS $$
DECLARE
  pruned bigint;
BEGIN
  PERFORM set_config('app.audit_pruning', '1', true);
  DELETE FROM audit_log WHERE created_at < older_than AND actor_id NOT LIKE 'demo-%';
  GET DIAGNOSTICS pruned = ROW_COUNT;
  RETURN pruned;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;
