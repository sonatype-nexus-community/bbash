BEGIN;

-- table: poll
CREATE TABLE poll
(
    poll_instance       varchar(255) PRIMARY KEY NOT NULL,
    last_polled_on      timestamp                NOT NULL,
    env_base_time       timestamp                NOT NULL,
    last_poll_completed timestamp                NOT NULL
);
INSERT INTO poll (poll_instance, last_polled_on, env_base_time, last_poll_completed)
VALUES ('1', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

COMMIT;
