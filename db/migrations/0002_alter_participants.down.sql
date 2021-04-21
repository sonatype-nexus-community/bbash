
BEGIN;

ALTER TABLE participants
    DROP COLUMN CampaignName;

COMMIT;
