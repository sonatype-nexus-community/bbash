BEGIN;

-- remove new campaign
DELETE FROM campaigns WHERE campaignname = 'cncf-2021-10-11';

-- restore old organizations from prior campaign
INSERT INTO organizations (Organization) VALUES ('thanos-io');
INSERT INTO organizations (Organization) VALUES ('cri-o');
INSERT INTO organizations (Organization) VALUES ('openebs');
INSERT INTO organizations (Organization) VALUES ('buildpacks');
INSERT INTO organizations (Organization) VALUES ('schemahero');

-- remove new organizations for this campaign
DELETE FROM organizations WHERE organization = 'harbor';
DELETE FROM organizations WHERE organization = 'kyverno';
DELETE FROM organizations WHERE organization = 'keptn';
DELETE FROM organizations WHERE organization = 'tikv';
DELETE FROM organizations WHERE organization = 'longhorn';
DELETE FROM organizations WHERE organization = 'kubevela';
DELETE FROM organizations WHERE organization = 'meshery';

COMMIT;
