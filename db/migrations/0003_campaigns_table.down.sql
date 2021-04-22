
BEGIN;

DROP TABLE IF EXISTS campaigns;

ALTER TABLE participants
    DROP COLUMN IF EXISTS Campaign;

ALTER TABLE participants
    ADD COLUMN CampaignName varchar(250) NOT NULL;

COMMIT;
