#!/bin/sh

set -e

load_secret_files() {
  # Save and restore IFS
  old_ifs="$IFS"
  IFS='
'
  # Capture all env variables starting with LISTMONK_ and ending with _FILE.
  # It's value is assumed to be a file path with its actual value.
  for line in $(env | grep '^LISTMONK_.*_FILE='); do
    var="${line%%=*}"
    fpath="${line#*=}"

    # If it's a valid file, read its contents and assign it to the var
    # without the _FILE suffix.
    # Eg: LISTMONK_DB_USER_FILE=/run/secrets/user -> LISTMONK_DB_USER=$(contents of /run/secrets/user)
    if [ -r "$fpath" ]; then
      new_var="${var%_FILE}"
      export "$new_var"="$(cat "$fpath")"
      unset "$var"
    else
      echo "Secret file for $var is not readable" >&2
      exit 1
    fi
  done
  IFS="$old_ifs"
}

# Load env variables from files if LISTMONK_*_FILE variables are set.
load_secret_files

echo "Launching MailView runtime as uid=$(id -u) gid=$(id -g)"
exec "$@"
