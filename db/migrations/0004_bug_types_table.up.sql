
BEGIN;

DROP TABLE IF EXISTS bugs;

CREATE TABLE bugs(
    ID UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category varchar(255) UNIQUE NOT NULL,
    pointValue int NOT NULL
);

COMMIT;