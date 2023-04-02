#!/usr/bin/env bash

device_id=$1
sites=/usr/local/etc/lehu/sites.txt
syncs=/home/st/sync

grep $device_id $sites | cut -d: -f1 | \
while read domain; do
        curl -s -H x-api-key:$api_key $host/rest/config/devices -d '{"deviceID":"'$device_id'"}'
	folder=/home/st/sync/$domain
	if [[ ! -d $folder ]]; then
		cp -R /home/st/sync/lehu.in $folder
		sed -E -i '/IMAP|ga_id|gt_id/d' $folder/env
	fi

	cat << EOF | curl -s -X PUT -H x-api-key:$api_key $host/rest/config/folders/$domain -d @-
	{
	  "id": "$domain",
	  "path": "$folder",
	  "type": "sendreceive",
	  "devices": [{
	    "deviceID": "$device_id"
	  }],
	  "fsWatcherEnabled": true,
	  "fsWatcherDelayS": 1
	}

	curl -s -X POST -H x-api-key:$api_key "$host/rest/db/ignores?folder=$domain" \
		--data-raw '{"ignore":["*.yml","*.htm"]}'
EOF
done
