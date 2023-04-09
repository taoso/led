#!/usr/bin/env bash

trap exit SIGINT SIGTERM

since=0
events=RemoteChangeDetected,PendingDevicesChanged

# 首次启动跳过已经发生的事件
curl -s -H x-api-key:$api_key "$host/rest/events?timeout=0&events=$events" > /tmp/events.txt
last=$(jq -r '.[]|.id' /tmp/events.txt | tail -n 1)
if [[ ! -z "$last" ]]; then
	since=$last
fi

while true; do
        curl -s -H x-api-key:$api_key "$host/rest/events?since=$since&events=$events" > /tmp/events.txt

	jq -r '.[]|select(.type == "PendingDevicesChanged")|.data.added[0].deviceID|select(.!=null)' \
		/tmp/events.txt | sort | uniq | \
		xargs -I % device.sh %

	# 如果删除 md 则同步删除对应的 htm/yml
	jq -r '.[]|select(.type == "RemoteChangeDetected")|select(.data.action == "deleted")|"\(.data.folder)/\(.data.path)"' \
		/tmp/events.txt | sort | uniq | grep -E "\.md$" | sed -E "s/\.md$//" | \
		xargs -I % rm -f ~/sync/%.{htm,yml}

	# 目前无法区分新建文件和修改文件，只能无脑触发更新索引页面
	jq -r '.[]|select(.type == "RemoteChangeDetected")|.data.folder' \
		/tmp/events.txt | sort | uniq | \
		xargs -I % build.sh %

	last=$(jq -r '.[]|.id' /tmp/events.txt | tail -n 1)
	if [[ ! -z "$last" ]]; then
		since=$last
	fi
done
