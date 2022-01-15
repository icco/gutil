#!/bin/sh
set -ex

SHA=$(git one)
for i in $(cat ./repos.txt); do
  echo $i
  tmp_dir=$(mktemp -d -t $i)
  gh repo clone $i $tmp_dir
  cd $tmp_dir
  go get -v -u -d github.com/icco/gutil@$SHA
  git diff
  git ci -a -m 'update gutil'
  go get -v -u -d  ./...
  go mod tidy -compat=1.17
  go build -v ./...
  git ci -a -m 'update'
  git push -u
done
