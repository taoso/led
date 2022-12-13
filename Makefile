MDs := $(shell find . -name '*.md')
HTMLs := $(MDs:.md=.html)
ROOT_DIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

export ROOT_DIR = $(ROOT_DIR)
export LUA_FILTER = $(ROOT_DIR)/description.lua

%.html: %.md ./head.tpl ./footer.tpl ./article.tpl
	pandoc -s -p \
		--toc \
		--template article.tpl \
		--metadata-file=index.yaml \
		--highlight-style=pygments \
		--lua-filter $(LUA_FILTER) \
		$< -o $@

index: ./head.tpl ./footer.tpl ./index.tpl
	find . -type d -exec index.sh {} \;

all: index $(HTMLs)
	feed.sh . > feed.xml
