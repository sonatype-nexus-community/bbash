BEGIN;

ALTER TABLE participants
    ALTER COLUMN Campaign DROP NOT NULL;

COMMIT;