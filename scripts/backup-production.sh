#!/usr/bin/env bash
set -euo pipefail

APP_DIR="${APP_DIR:-/opt/imageCreate}"
BACKUP_DIR="${BACKUP_DIR:-$APP_DIR/backups}"
RETENTION_DAYS="${RETENTION_DAYS:-14}"
TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"

cd "$APP_DIR"
mkdir -p "$BACKUP_DIR"
chmod 700 "$BACKUP_DIR"

db_backup="$BACKUP_DIR/postgres-$TIMESTAMP.sql.gz"
images_backup="$BACKUP_DIR/images-$TIMESTAMP.tar.gz"

docker compose exec -T postgres pg_dump -U postgres -d imagecreate | gzip -9 > "$db_backup"

docker run --rm \
  -v imagecreate_images:/images:ro \
  alpine:3.20 \
  tar -czf - -C /images . > "$images_backup"

find "$BACKUP_DIR" -type f \( -name "postgres-*.sql.gz" -o -name "images-*.tar.gz" \) -mtime +"$RETENTION_DAYS" -delete

printf 'backup_created db=%s images=%s\n' "$db_backup" "$images_backup"
