MDs := $(shell find . -name '*.md')
DIRs := $(shell find . -type d)
HTMLs := $(MDs:.md=.htm)
METAs := $(MDs:.md=.yml)
INDXs := $(addsuffix /index.htm,$(DIRs))

PWD := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

export ROOT_DIR = $(PWD)
export LUA_FILTER = $(PWD)/desc.lua

.PRECIOUS: %.yml

%.yml: %.md
	meta.sh $< > $@

# 没找到规则同时匹配 index.htm 和子目录的 index.htm 😂先写两遍
%/index.htm: ./head.tpl ./footer.tpl ./index.tpl $(METAs)
	index.sh $@

index.htm: ./head.tpl ./footer.tpl ./index.tpl $(METAs)
	index.sh $@

%.htm: %.md ./head.tpl ./footer.tpl ./article.tpl
	pandoc -s -p --wrap=none \
		--toc \
		--template article.tpl \
		--metadata-file=index.yml \
		--highlight-style=pygments \
		--lua-filter $(LUA_FILTER) \
		--from markdown+east_asian_line_breaks \
		$< -o $@

all: $(INDXs) $(HTMLs)
	feed.sh . > feed.xml
	map.sh . > map.xml

.PHONY: $(INDXs)
