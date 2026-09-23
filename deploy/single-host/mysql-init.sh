#!/bin/sh
set -eu
# prepare.py generates hex-only passwords; reject SQL metacharacters.
for password in "$BACKEND_DB_PASSWORD" "$BRAIN_DB_PASSWORD"; do
  case "$password" in
    ''|*[!0-9a-f]*) echo 'Database passwords must be generated hex strings' >&2; exit 1 ;;
  esac
done
MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql --protocol=socket -uroot <<SQL
CREATE DATABASE genio_backend CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE DATABASE ai_brain CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER 'backend'@'%' IDENTIFIED BY '${BACKEND_DB_PASSWORD}';
CREATE USER 'brain'@'%' IDENTIFIED BY '${BRAIN_DB_PASSWORD}';
GRANT ALL PRIVILEGES ON genio_backend.* TO 'backend'@'%';
GRANT ALL PRIVILEGES ON ai_brain.* TO 'brain'@'%';
SQL
