#!/usr/bin/env bash
set -euo pipefail

# Stop the app before running this script. DATA_DIR matches the app's setting;
# without it, reset the data directory next to this script from any working dir.
app_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
data_dir="${DATA_DIR:-$app_root/data}"
if [[ ! -e "$data_dir" ]]; then
    printf 'No app data to clean.\n'
    exit 0
fi
data_dir="$(cd -- "$data_dir" && pwd -P)"
case "$data_dir" in
    /|[A-Za-z]:/|/[A-Za-z]) printf 'Refusing to clean a filesystem root.\n' >&2; exit 1 ;;
esac

# All deletion targets are fixed children of the resolved data directory.
uploads="$data_dir/uploads"
[[ "$uploads" == "$data_dir/"* && "${uploads##*/}" == uploads ]] || exit 1
rm -f -- "$data_dir/bracket.db" "$data_dir/bracket.db-wal" "$data_dir/bracket.db-shm" "$data_dir/bracket.db-journal"
rm -rf -- "$uploads"
printf 'Reset app database and uploads in %s\n' "$data_dir"
