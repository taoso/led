MDs := $(shell find . -name '*.md')
HTMLs := $(MDs:.md=.html)
ROOT_DIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

%.html: %.md ./head.tpl ./article.tpl
	pandoc -s -p --from gfm --highlight-style=pygments \
		--template article.tpl \
		--lua-filter $(ROOT_DIR)/description.lua \
		$< -o $@

all: $(HTMLs)
	find . -type d ! -path '*.assets' -exec index.sh {} \;
	feed.sh . > feed.xml
