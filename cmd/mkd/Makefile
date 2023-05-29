MDs := $(shell find . -name '*.md')
HTMLs := $(MDs:.md=.htm)

PWD := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

export ROOT_DIR = $(PWD)
export LUA_FILTER = $(PWD)/desc.lua

.PRECIOUS: %.yml

%.yml: %.md
	meta.sh $< > $@

%.htm: %.md %.yml ./head.tpl ./footer.tpl ./article.tpl
	pandoc -s -p --wrap=none \
		--toc \
		--template article.tpl \
		--metadata-file=index.yml \
		--highlight-style=pygments \
		--lua-filter $(LUA_FILTER) \
		--from markdown+east_asian_line_breaks \
		$< -o $@

all: $(HTMLs)
	index.sh .
	feed.sh . > feed.xml
	map.sh . > map.xml
