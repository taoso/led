MDs := $(shell find . -name '*.md')
HTMLs := $(MDs:.md=.html)
ROOT_DIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

%.html: %.md ./head.tpl ./footer.tpl ./article.tpl
	pandoc -s -p --highlight-style=pygments \
		--template article.tpl \
		--metadata-file=index.yaml \
		--lua-filter $(ROOT_DIR)/description.lua \
		$< -o $@

index: ./head.tpl ./footer.tpl ./index.tpl
	find . -type d ! -path '*.assets' -exec index.sh {} \;

all: index $(HTMLs)
	feed.sh . > feed.xml
