#!/usr/bin/env bash

device_id=$1

grep $device_id /usr/local/etc/lehu/sites.txt | cut -d: -f1 | while read domain; do
        curl -s -H x-api-key:$api_key $host/rest/config/devices -d '{"deviceID":"'$device_id'"}'
	folder=~/sync/$domain
	if [[ ! -d $folder ]]; then
		cp -R ~/sync/lehu.zz.ac $folder
		sed -E -i "s/lehu\.in/$domain" $folder/env
	fi

	fdata=$(curl -s -H x-api-key:$api_key $host/rest/config/folders/$domain)
	if [[ "$fdata" != "No folder with given ID" ]]; then
		devices=$(echo $fdata | jq "{devices}|.devices+=[{\"deviceID\":\"$device_id\"}]")
		echo $devices | curl -s -X PATCH -H x-api-key:$api_key $host/rest/config/folders/$domain -d @-
		continue
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
EOF

	cat << EOF | curl -s -X POST -H x-api-key:$api_key "$host/rest/db/ignores?folder=$domain" -d @-
	{"ignore":[
	     "#*#",
	     "*.autosave.*"
	     "*.swp",
	     "*.tmp",
	     "*.yml",
	     "*.[0-9]*.svg",
	     ".#*",
	     ".DS_Store",
	     "feed.xml",
	     "~$*",
	]}
EOF
done
