BEGIN;

DROP TABLE IF EXISTS organizations;

CREATE TABLE organizations (
    Id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    Organization TEXT UNIQUE NOT NULL
);

INSERT INTO organizations (Organization) VALUES ('thanos-io');
INSERT INTO organizations (Organization) VALUES ('serverlessworkflow');
INSERT INTO organizations (Organization) VALUES ('chaos-mesh');
INSERT INTO organizations (Organization) VALUES ('cri-o');
INSERT INTO organizations (Organization) VALUES ('openebs');
INSERT INTO organizations (Organization) VALUES ('buildpacks');
INSERT INTO organizations (Organization) VALUES ('schemahero');
INSERT INTO organizations (Organization) VALUES ('sonatype-nexus-community');

COMMIT;
