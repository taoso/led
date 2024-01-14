#!/usr/bin/env bash

set -e

cd ~/sync/$1/

# https://stackoverflow.com/a/50673471
set -o allexport
source ./env
set +o allexport

make -f /usr/local/opt/led/cmd/mkd/Makefile all
