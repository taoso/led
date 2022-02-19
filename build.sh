#!/usr/bin/env bash

# 构建整个站点，整个系统目标结构如下：
# ~ +- sync
#      - site1.com
#   -- www
#      - site1.com

site=$1
[[ -z "$site" ]] && exit 1

rsync -a --delete --exclude='.*' --exclude='*.html' ~/sync/$site/ ~/www/$site/

cd ~/www/$site/
make -f ~/lehu-sh/Makefile all
cp ~/sync/$site/*.html ~/www/$site/ 2>/dev/null

# 清理 sync 中已经删除的文件夹
diff <(find ~/sync/$site/ -type d ! -path '*/.*'|sed -e 's#/sync/#/www/#') <(find ~/www/$site/ -type d) |\
grep '>'|cut -d' ' -f2 | xargs -I % rm -r %
