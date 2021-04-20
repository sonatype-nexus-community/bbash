
BEGIN;

CREATE EXTENSION pgcrypto;

CREATE TABLE teams(
    Id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    TeamName varchar(250) NOT NULL UNIQUE,
    Organization varchar(250)
);

CREATE TABLE participants(
    Id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    GitHubName varchar(250) NOT NULL UNIQUE,
    Email varchar(250),
    DisplayName varchar(250),
    Score int,
    fk_team UUID references teams (Id),
    JoinedAt timestamp NOT NULL DEFAULT NOW()
);

COMMIT;
