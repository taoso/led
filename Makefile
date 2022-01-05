MDs := $(shell find . -name '*.md')
HTMLs := $(MDs:.md=.html)

%.html: %.md ./head.tpl ./article.tpl
	pandoc -s -p --highlight-style=pygments \
		--template article.tpl $< -o $@

all: $(HTMLs)
	find . -type d ! -path '*.assets' -exec index.sh {} \;
	feed.sh . > feed.xml
