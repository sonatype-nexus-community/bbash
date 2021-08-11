BEGIN;

ALTER TABLE participants
    ADD COLUMN UpstreamId varchar(250) NOT NULL;
    
COMMIT;