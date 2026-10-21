#!/bin/sh
set -eu

check_dir=$(mktemp -d)
trap 'rm -rf "$check_dir"' EXIT

for database in workspace identity; do
    url=http://localhost:8083/
    if [ "$database" = identity ]; then
        url='http://localhost:8083/?local=identity'
    fi

    result=$(curl --silent --show-error --location \
        --cookie "$check_dir/$database.cookies" \
        --cookie-jar "$check_dir/$database.cookies" \
        --output "$check_dir/$database.html" \
        --write-out '%{http_code} %{url_effective}' "$url")
    case "$result" in
        "200 "*"pgsql=$database-postgres&username=$database&db=$database"*) ;;
        *) printf '%s automatic login failed: %s\n' "$database" "$result" >&2; exit 1 ;;
    esac
    grep -q 'Logout' "$check_dir/$database.html"
    printf '%s automatic login passed\n' "$database"
done

status=$(curl --silent --output /dev/null --write-out '%{http_code}' 'http://localhost:8083/?local=unknown')
[ "$status" = 400 ]
