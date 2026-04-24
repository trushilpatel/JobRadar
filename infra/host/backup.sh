#!/usr/bin/env bash
# Nightly restic backup wrapper. Initialises the repo on first run, then
# backs up each path in BACKUP_PATHS, then prunes per BACKUP_RETENTION.
#
# Required env:
#   BACKUP_REPO            -- restic repo URL (e.g. s3:https://s3.example.com/bucket
#                             or /mnt/backup/restic)
#   BACKUP_PASSWORD_FILE   -- file containing restic repo password (chmod 600)
#   BACKUP_PATHS           -- space-separated absolute paths to back up
#
# Optional env:
#   BACKUP_RETENTION       (default 30d)
#   BACKUP_LOG_DIR         (default /var/log/jobradar-backup)

set -euo pipefail

: "${BACKUP_REPO:?BACKUP_REPO is required}"
: "${BACKUP_PASSWORD_FILE:?BACKUP_PASSWORD_FILE is required}"
: "${BACKUP_PATHS:?BACKUP_PATHS is required (space-separated paths)}"
BACKUP_RETENTION="${BACKUP_RETENTION:-30d}"
BACKUP_LOG_DIR="${BACKUP_LOG_DIR:-/var/log/jobradar-backup}"

if [[ ! -r "$BACKUP_PASSWORD_FILE" ]]; then
	echo "BACKUP_PASSWORD_FILE '$BACKUP_PASSWORD_FILE' not readable" >&2
	exit 1
fi

install -d -m 0750 "$BACKUP_LOG_DIR"
LOG_FILE="$BACKUP_LOG_DIR/backup-$(date -u +%Y%m%dT%H%M%SZ).log"

export RESTIC_REPOSITORY="$BACKUP_REPO"
export RESTIC_PASSWORD_FILE="$BACKUP_PASSWORD_FILE"

log() { printf '[backup] %s\n' "$*" | tee -a "$LOG_FILE"; }

log "starting backup to $BACKUP_REPO"

# Initialise repo if it doesn't exist yet.
if ! restic snapshots >/dev/null 2>&1; then
	log "initialising restic repo"
	restic init | tee -a "$LOG_FILE"
fi

# Validate paths exist before running (restic would skip silently otherwise).
paths_to_backup=()
for p in $BACKUP_PATHS; do
	if [[ -e "$p" ]]; then
		paths_to_backup+=("$p")
	else
		log "WARN: path does not exist, skipping: $p"
	fi
done

if [[ ${#paths_to_backup[@]} -eq 0 ]]; then
	log "no valid paths to back up; exiting"
	exit 1
fi

log "backing up: ${paths_to_backup[*]}"
restic backup --tag jobradar "${paths_to_backup[@]}" 2>&1 | tee -a "$LOG_FILE"

log "pruning snapshots older than $BACKUP_RETENTION"
restic forget --keep-within "$BACKUP_RETENTION" --prune 2>&1 | tee -a "$LOG_FILE"

log "done"
