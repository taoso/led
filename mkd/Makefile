MDs := $(shell find . -name '*.md')
HTMLs := $(MDs:.md=.htm)
PWD := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

export ROOT_DIR = $(PWD)
export LUA_FILTER = $(PWD)/desc.lua

%.htm: %.md ./head.tpl ./footer.tpl ./article.tpl
	pandoc -s -p --wrap=none \
		--toc \
		--template article.tpl \
		--metadata-file=index.yaml \
		--highlight-style=pygments \
		--lua-filter $(LUA_FILTER) \
		--from gfm+east_asian_line_breaks \
		$< -o $@

index: ./head.tpl ./footer.tpl ./index.tpl
	find . -type d -exec index.sh {} \;

all: index $(HTMLs)
	feed.sh . > feed.xml
