-- One row per pass: an aircraft's positions with no gap of more than ten minutes between them (README: History).
-- Reads the `positions` view (one row per position event) and the variables home_lat and home_lon.
-- DuckDB dialect; M3 runs the same model on Snowflake over the table Snowpipe loads.
WITH marked AS (
    SELECT
        *,
        CASE
            WHEN lag("at") OVER w IS NULL OR "at" - lag("at") OVER w > INTERVAL 10 MINUTE THEN 1
            ELSE 0
        END AS starts_pass
    FROM positions
    WINDOW w AS (PARTITION BY hex ORDER BY "at")
),
numbered AS (
    SELECT
        *,
        sum(starts_pass) OVER (PARTITION BY hex ORDER BY "at" ROWS UNBOUNDED PRECEDING) AS pass,
        -- Great-circle distance from home, in km (haversine, mean Earth radius).
        2 * 6371.0088 * asin(sqrt(
            pow(sin(radians(lat - getvariable('home_lat')) / 2), 2)
            + cos(radians(getvariable('home_lat'))) * cos(radians(lat))
              * pow(sin(radians(lon - getvariable('home_lon')) / 2), 2)
        )) AS distance_km
    FROM marked
)
SELECT
    hex,
    pass,
    min("at") AS first_seen,
    max("at") AS last_seen,
    mode(callsign) AS callsign,
    mode(registration) AS registration,
    mode(type_code) AS type_code,
    min(distance_km) AS closest_km,
    min(alt_ft) FILTER (WHERE NOT coalesce(on_ground, false)) AS lowest_alt_ft,
    bool_or(coalesce(on_ground, false)) AS on_ground_at_some_point,
    bool_or(coalesce(military, false)) AS military,
    bool_or(coalesce(private, false)) AS private,
    count(*) AS positions
FROM numbered
GROUP BY hex, pass
ORDER BY first_seen, hex
