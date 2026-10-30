-- M3's "Done when": Snowflake's counts reconcile with the poller's own. Compare each run's row with the line
-- `overhead verify` prints for it ("<run>: N events, up to seq M"): EVENTS = N, LAST_SEQ = M, MISSING = 0.
SELECT RUN,
    COUNT(DISTINCT SEQ)                         AS EVENTS,
    MAX(SEQ)                                    AS LAST_SEQ,
    MAX(SEQ) - COUNT(DISTINCT SEQ)              AS MISSING,
    COUNT(*) - COUNT(DISTINCT SEQ)              AS LOADED_TWICE,
    MIN(PUBLISHED)                              AS FROM_TIME,
    MAX(PUBLISHED)                              AS TO_TIME
FROM OVERHEAD.RAW.POSITIONS
GROUP BY RUN
ORDER BY FROM_TIME;
