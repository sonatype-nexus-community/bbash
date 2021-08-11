BEGIN;

ALTER TABLE participants
    DROP COLUMN UpstreamId;

COMMIT;