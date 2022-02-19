#!/usr/bin/env bash

trap exit SIGINT SIGTERM

since=0

# 首次启动跳过已经发生的事件
curl -s -H x-api-key:$api_key $host/rest/events?timeout=0 > /tmp/events.txt
last=$(jq -r '.[]|.id' /tmp/events.txt | tail -n 1)
if [[ ! -z "$last" ]]; then
	since=$last
fi

while true; do
        curl -s -H x-api-key:$api_key $host/rest/events?since=$since > /tmp/events.txt

	jq -r '.[]|select(.type == "PendingDevicesChanged")|.data.added[0].deviceID|select(.!=null)' /tmp/events.txt | sort | uniq | xargs -I % device.sh %
	jq -r '.[]|select(.type == "FolderCompletion")|.data.folder' /tmp/events.txt | sort | uniq | xargs -I % build.sh %

	last=$(jq -r '.[]|.id' /tmp/events.txt | tail -n 1)
	if [[ ! -z "$last" ]]; then
		since=$last
	fi
done
