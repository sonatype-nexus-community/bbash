
BEGIN;

ALTER TABLE participants
    ADD COLUMN CampaignName varchar(250) NOT NULL UNIQUE;

COMMIT;
