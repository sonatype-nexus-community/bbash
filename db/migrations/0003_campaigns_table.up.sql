
BEGIN;

DROP TABLE IF EXISTS campaigns;

CREATE TABLE campaigns(
    ID UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    CampaignName varchar(250) NOT NULL UNIQUE
);

ALTER TABLE participants
    DROP COLUMN IF EXISTS CampaignName;

ALTER TABLE participants
    ADD COLUMN Campaign UUID references campaigns (Id);

COMMIT;
