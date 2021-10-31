#!/usr/bin/env bash

trap exit SIGINT SIGTERM

since=0

while true; do
        curl -s -H x-api-key:$api_key $host/rest/events?since=$since |\
                jq '.[]|"\(.id),\(.type),\(.data.folder)"' |\
                sed 's/"//g' > /tmp/events.txt

        if [[ -s /tmp/events.txt ]]; then
                since=$(tail -n 1 /tmp/events.txt | cut -d, -f1)
        fi
        grep FolderCompletion /tmp/events.txt| cut -d, -f3 | sort | uniq | xargs -I % build.sh %
done
