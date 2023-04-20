#!/usr/bin/env bash

set -e

cd ~/sync/$1/

# https://stackoverflow.com/a/50673471
set -o allexport
source ./env
set +o allexport

make -f ~/led/cmd/mkd/Makefile all
