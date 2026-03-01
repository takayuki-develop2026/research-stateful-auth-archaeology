BEGIN;

-- drop and recreate CHECK with published added
ALTER TABLE agent_v10.routing_proposals_v10
  DROP CONSTRAINT IF EXISTS routing_proposals_v10_status_ck;

ALTER TABLE agent_v10.routing_proposals_v10
  ADD CONSTRAINT routing_proposals_v10_status_ck
    CHECK (status IN ('draft','evaluating','ready_for_review','approved','rejected','published','review_required','failed'));

COMMIT;