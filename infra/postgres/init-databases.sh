#!/bin/bash
# Creates additional databases on first boot of the consolidated Postgres container.
set -euo pipefail

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    SELECT 'CREATE DATABASE products' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'products')\gexec
    SELECT 'CREATE DATABASE orders' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'orders')\gexec
    SELECT 'CREATE DATABASE cart' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'cart')\gexec
    SELECT 'CREATE DATABASE payments' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'payments')\gexec
    SELECT 'CREATE DATABASE notifications' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'notifications')\gexec
EOSQL
