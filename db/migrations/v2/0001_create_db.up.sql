
BEGIN;


-- table: campaign
CREATE TABLE campaign(
    ID UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name varchar(250) NOT NULL UNIQUE,
    created_on timestamp NOT NULL DEFAULT NOW(),
    create_order SERIAL,
    active boolean DEFAULT false,
    upstream_id varchar(250) NOT NULL,
    note TEXT
);


-- table: source_control_provider
CREATE TABLE source_control_provider(
    Id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT UNIQUE NOT NULL,
    url TEXT UNIQUE NOT NULL
);
-- add scp providers
INSERT INTO source_control_provider (name,url) VALUES ('GitHub','https://github.com') ON CONFLICT DO NOTHING;
INSERT INTO source_control_provider (name,url) VALUES ('GitLab','https://gitlab.com') ON CONFLICT DO NOTHING;
INSERT INTO source_control_provider (name,url) VALUES ('Bitbucket','https://bitbucket.org/') ON CONFLICT DO NOTHING;


-- table: organization
CREATE TABLE organization (
    Id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fk_scp UUID references source_control_provider (Id) NOT NULL,
    Organization TEXT NOT NULL,
    unique (fk_scp, Organization)
);


-- table: team
CREATE TABLE team(
    Id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fk_campaign UUID references campaign (Id) NOT NULL,
    TeamName varchar(250) NOT NULL,
    unique (fk_campaign, TeamName)
);


-- table: bug
CREATE TABLE bug(
     ID UUID PRIMARY KEY DEFAULT gen_random_uuid(),
     fk_campaign UUID references campaign (Id) NOT NULL,
     category varchar(255) NOT NULL,
     pointValue int NOT NULL,
     unique (fk_campaign, category)
);


-- table: participant
CREATE TABLE participant(
    Id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fk_campaign UUID references campaign (Id) NOT NULL,
    fk_scp UUID references source_control_provider (Id) NOT NULL,
    login_name varchar(250) NOT NULL,
    Email varchar(250),
    DisplayName varchar(250),
    Score int,
    fk_team UUID references team (Id),
    JoinedAt timestamp NOT NULL DEFAULT NOW(),
    UpstreamId varchar(250) NOT NULL,
    unique (fk_campaign, fk_scp, login_name)
);


-- table: scoring_event
CREATE TABLE scoring_event (
    fk_campaign UUID references campaign (Id) NOT NULL,
    fk_scp UUID references source_control_provider (Id),
    repoOwner TEXT NOT NULL,
    repoName TEXT NOT NULL,
    pr INT,
    username TEXT NOT NULL,
    points INT NOT NULL,
    primary key (fk_campaign, fk_scp, repoOwner, repoName, pr)
);


COMMIT;
