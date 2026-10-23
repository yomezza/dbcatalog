#!/usr/bin/env python3
"""Export the reviewed 009 -> current psql upgrade, with the same immutable ledger."""
from pathlib import Path
import hashlib

root = Path(__file__).resolve().parents[1]
migrations = []
for path in sorted((root / "internal/database/migrations").glob("*.sql")):
    version = int(path.name.split("_", 1)[0])
    raw = path.read_bytes()
    migrations.append((version, path.name, hashlib.sha256(raw).hexdigest(), raw.decode("utf-8")))
latest = max(row[0] for row in migrations)
baseline = 9
target = root / f"downloads/CatalogDB-upgrade-{baseline:03d}-to-{latest:03d}.sql"
out = [
    "-- CatalogDB: transactional upgrade from schema 009. Existing rows and history are preserved.",
    "-- Run with psql -X -v catalog_schema=YOUR_SCHEMA -f THIS_FILE against a backed-up database.",
    "-- No old migration is modified. The application also embeds and applies these migrations.",
    r"\set ON_ERROR_STOP on",
    r"\if :{?catalog_schema}",
    r"\else",
    r"\set catalog_schema dbcatalog",
    r"\endif",
    "BEGIN;",
    "SET LOCAL statement_timeout='10min';",
    "SET LOCAL lock_timeout='30s';",
    "SET LOCAL catalogdb.protocol='2';",
    "SELECT pg_advisory_xact_lock(724194301112);",
    "SELECT set_config('catalogdb.migration_schema',:'catalog_schema',true);",
    "SELECT set_config('search_path',format('%I,pg_catalog,pg_temp',:'catalog_schema'),true);",
    "DO $catalog_check$ BEGIN",
    "IF current_schema() IS DISTINCT FROM current_setting('catalogdb.migration_schema') OR to_regclass('schema_migrations') IS NULL THEN RAISE EXCEPTION 'The existing application schema is required'; END IF;",
    "IF EXISTS(SELECT 1 FROM schema_migrations m LEFT JOIN(VALUES",
]
values = ",\n".join(f"({v},'{checksum}')" for v, _, checksum, _ in migrations)
out += [
    values,
    ")e(version,checksum)ON e.version=m.version WHERE e.version IS NULL OR e.checksum<>m.checksum) THEN RAISE EXCEPTION 'Unknown migration or checksum mismatch';END IF;",
    f"IF(SELECT count(*) FROM schema_migrations WHERE version BETWEEN 1 AND {baseline})<>{baseline} THEN RAISE EXCEPTION 'Apply original migrations 001-009 first';END IF;",
    "IF EXISTS(SELECT 1 FROM generate_series(1,(SELECT max(version)::int FROM schema_migrations))g WHERE NOT EXISTS(SELECT 1 FROM schema_migrations m WHERE m.version=g)) THEN RAISE EXCEPTION 'Migration ledger contains a gap';END IF;",
    "END $catalog_check$;",
]
for version, name, checksum, source in migrations:
    if version <= baseline:
        continue
    assert "$catalog_step$" not in source and "$catalog_source$" not in source
    out += [
        f"-- {name}",
        "DO $catalog_step$ BEGIN",
        f"IF NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version={version}) THEN",
        "EXECUTE $catalog_source$",
        source,
        "$catalog_source$;",
        f"INSERT INTO schema_migrations(version,name,checksum)VALUES({version},'{name}','{checksum}');",
        "END IF; END $catalog_step$;",
    ]
out += ["COMMIT;", "-- Upgrade complete. Re-running this file checks the ledger and skips applied steps."]
target.write_bytes(("\n".join(out) + "\n").encode("utf-8"))
digest = hashlib.sha256(target.read_bytes()).hexdigest()
Path(str(target) + ".sha256").write_bytes((digest + "  " + target.name + "\n").encode("ascii"))
print(target)
