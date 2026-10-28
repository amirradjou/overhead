# overhead

The planes above home, live: polls free community ADS-B feeds for a radius around home, enriches each aircraft, keeps the history in Snowflake, and shows it on a TV-sized Next.js screen with alerts for rare visitors.

## Getting started

```sh
go mod download
make build && ./bin/overhead version
```

## Development

```sh
make test     # tests
make lint     # lint + format
```
