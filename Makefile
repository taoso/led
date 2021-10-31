MDs := $(shell find . -name '*.md')
HTMLs := $(MDs:.md=.html)

%.html: %.md article.tpl Makefile
	pandoc -s -p --highlight-style=pygments \
		--template article.tpl $< -o $@

all: $(HTMLs)
	find . -type d ! -path '*.assets' -exec index.sh {} \;
	feed.sh . > feed.xml
	map.sh . > map.xml
