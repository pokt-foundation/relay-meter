CREATE SEQUENCE success_seq START 1;
CREATE SEQUENCE error_seq START 1;

CREATE TABLE http_source_relay_count (
    app_public_key char(64) NOT NULL,
    day date NOT NULL,
    success BIGINT DEFAULT nextval('success_seq') NOT NULL,
    error BIGINT DEFAULT nextval('error_seq') NOT NULL,
    PRIMARY KEY (app_public_key, day)
);
