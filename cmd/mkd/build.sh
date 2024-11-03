#!/usr/bin/env bash

set -e

cd ~/sync/$1/

# https://stackoverflow.com/a/50673471
set -o allexport
source ./env
set +o allexport

export site_url=https://$1
export author_url=/about.html

make -f /usr/local/opt/led/cmd/mkd/Makefile all
