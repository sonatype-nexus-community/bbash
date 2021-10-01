BEGIN;

-- add new campaign
INSERT INTO campaigns (campaignname) VALUES ('cncf-2021-10-11') ON CONFLICT DO NOTHING;

-- remove old organizations from prior campaign
DELETE FROM organizations WHERE organization = 'thanos-io';
DELETE FROM organizations WHERE organization = 'cri-o';
DELETE FROM organizations WHERE organization = 'openebs';
DELETE FROM organizations WHERE organization = 'buildpacks';
DELETE FROM organizations WHERE organization = 'schemahero';

-- add new organizations for this campaign
INSERT INTO organizations (Organization) VALUES ('harbor') ON CONFLICT DO NOTHING;
INSERT INTO organizations (Organization) VALUES ('kyverno') ON CONFLICT DO NOTHING;
INSERT INTO organizations (Organization) VALUES ('keptn') ON CONFLICT DO NOTHING;
INSERT INTO organizations (Organization) VALUES ('tikv') ON CONFLICT DO NOTHING;
INSERT INTO organizations (Organization) VALUES ('longhorn') ON CONFLICT DO NOTHING;
INSERT INTO organizations (Organization) VALUES ('kubevela') ON CONFLICT DO NOTHING;
INSERT INTO organizations (Organization) VALUES ('meshery') ON CONFLICT DO NOTHING;

COMMIT;
