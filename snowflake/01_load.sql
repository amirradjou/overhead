-- overhead history in Snowflake (docs/SPEC.md: History, M3). Run once, in order, as ACCOUNTADMIN on the trial; M6's
-- Terraform makes the same objects. Replace <bucket>, <prefix> and <role-arn> with the values Terraform prints.
-- Cost guard: one X-Small warehouse that suspends after a minute idle.

CREATE DATABASE IF NOT EXISTS OVERHEAD;
CREATE SCHEMA IF NOT EXISTS OVERHEAD.RAW;
CREATE SCHEMA IF NOT EXISTS OVERHEAD.ANALYTICS;
CREATE WAREHOUSE IF NOT EXISTS OVERHEAD_XS WAREHOUSE_SIZE = XSMALL AUTO_SUSPEND = 60 AUTO_RESUME = TRUE
    INITIALLY_SUSPENDED = TRUE;

-- Snowflake reads the bucket through an IAM role it assumes; Terraform makes the role and trusts Snowflake's user.
CREATE STORAGE INTEGRATION IF NOT EXISTS OVERHEAD_S3
    TYPE = EXTERNAL_STAGE
    STORAGE_PROVIDER = 'S3'
    ENABLED = TRUE
    STORAGE_AWS_ROLE_ARN = '<role-arn>'
    STORAGE_ALLOWED_LOCATIONS = ('s3://<bucket>/<prefix>/positions/');

USE SCHEMA OVERHEAD.RAW;

-- The history writer's batches: one JSON object per line, gzipped.
CREATE FILE FORMAT IF NOT EXISTS JSONL_GZ TYPE = JSON COMPRESSION = GZIP STRIP_OUTER_ARRAY = FALSE;

CREATE STAGE IF NOT EXISTS POSITIONS_STAGE
    URL = 's3://<bucket>/<prefix>/positions/'
    STORAGE_INTEGRATION = OVERHEAD_S3
    FILE_FORMAT = JSONL_GZ;

-- One row per position event. RUN and SEQ identify an event; FILE records which batch it came from, so a batch
-- written twice (a redelivery) can be seen and counted once.
CREATE TABLE IF NOT EXISTS POSITIONS (
    RUN          STRING NOT NULL,
    SEQ          NUMBER NOT NULL,
    PUBLISHED    TIMESTAMP_TZ,
    HEX          STRING,
    CALLSIGN     STRING,
    REGISTRATION STRING,
    TYPE_CODE    STRING,
    SQUAWK       STRING,
    EMERGENCY    STRING,
    LAT          FLOAT,
    LON          FLOAT,
    ALT_FT       NUMBER,
    ON_GROUND    BOOLEAN,
    SPEED_KT     FLOAT,
    TRACK_DEG    FLOAT,
    VERT_RATE    NUMBER,
    MILITARY     BOOLEAN,
    PRIVATE      BOOLEAN,
    SOURCE       STRING,
    AT           TIMESTAMP_TZ,
    FILE         STRING,
    LOADED       TIMESTAMP_TZ DEFAULT CURRENT_TIMESTAMP()
);

-- Snowpipe loads each new batch as S3 announces it (the bucket's event notification goes to the pipe's queue).
CREATE PIPE IF NOT EXISTS POSITIONS_PIPE AUTO_INGEST = TRUE AS
COPY INTO POSITIONS (RUN, SEQ, PUBLISHED, HEX, CALLSIGN, REGISTRATION, TYPE_CODE, SQUAWK, EMERGENCY, LAT, LON, ALT_FT,
                     ON_GROUND, SPEED_KT, TRACK_DEG, VERT_RATE, MILITARY, PRIVATE, SOURCE, AT, FILE)
FROM (
    SELECT $1:run::STRING, $1:seq::NUMBER, $1:published::TIMESTAMP_TZ, $1:hex::STRING, $1:callsign::STRING,
           $1:registration::STRING, $1:type_code::STRING, $1:squawk::STRING, $1:emergency::STRING,
           $1:lat::FLOAT, $1:lon::FLOAT, $1:alt_ft::NUMBER, COALESCE($1:on_ground::BOOLEAN, FALSE),
           $1:speed_kt::FLOAT, $1:track_deg::FLOAT, $1:vert_rate_fpm::NUMBER,
           COALESCE($1:military::BOOLEAN, FALSE), COALESCE($1:private::BOOLEAN, FALSE),
           $1:source::STRING, $1:at::TIMESTAMP_TZ, METADATA$FILENAME
    FROM @POSITIONS_STAGE
);

-- The ARN to put on the bucket's event notification:
SHOW PIPES LIKE 'POSITIONS_PIPE';
