#!/usr/bin/env bash

set -e

cd ~/sync/$1/
make -f ~/lehu-sh/Makefile all
