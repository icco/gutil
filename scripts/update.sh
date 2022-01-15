#!/bin/sh

SHA=$(git one)
for i in $(cat ./repos.txt); do
  echo $i
done
