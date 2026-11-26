#!/usr/bin/env bash

set -e

# build.sh 可能通过软链接（如 /usr/local/opt/led）或相对路径被调用，
# 先解析出脚本自己所在目录，后面一律用它，不再写死安装路径。
mkd=$(cd "$(dirname "$0")" && pwd -P)

cd "$HOME/sync/$1/"

export site_title=$1

# https://stackoverflow.com/a/50673471
set -o allexport
[ -f ./env ] && source ./env
set +o allexport

export site_url=https://$1
export author_url=/about.html

make -f "$mkd/Makefile" all
