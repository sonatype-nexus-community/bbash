BEGIN;

DROP TABLE IF EXISTS scoring_events;

CREATE TABLE scoring_events (
    repoOwner TEXT NOT NULL,
    repoName TEXT NOT NULL,
    pr INT,
    username TEXT NOT NULL,
    points INT NOT NULL,
    primary key (repoOwner, repoName, pr)
);

COMMIT;