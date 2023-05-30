MDs := $(shell find . -name '*.md')
HTMLs := $(MDs:.md=.htm)
METAs := $(MDs:.md=.yml)

PWD := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

export ROOT_DIR = $(PWD)
export LUA_FILTER = $(PWD)/desc.lua

%.yml: %.md
	$(PWD)/meta.sh $< > $@

%.htm: %.md ./head.tpl ./footer.tpl ./article.tpl
	pandoc -s -p --wrap=none \
		--toc \
		--template article.tpl \
		--highlight-style=pygments \
		--lua-filter $(LUA_FILTER) \
		--from markdown+east_asian_line_breaks \
		$< -o $@

index: $(METAs)
	$(PWD)/index.sh .
	$(PWD)/feed.sh .

all: index $(HTMLs)

clean:
	rm -f $(METAs)
	rm -f $(HTMLs)
	find . \( -name 'index.yml' -o -name 'index.htm' \) -type f -exec rm {} \;
