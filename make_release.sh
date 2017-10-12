#!/bin/sh
# Prepare binary *nix release:
release="release"
echo $#
if [ $# -eq 1 ]; then
    release="$1"
fi
mkdir "$release"
cp static/* "$release/" # avoids copying hidden files
#rm "$release/robots.txt" # not necessary for local use
cp README.md "$release/"
go build -o "$release/ais" server/*.go
tar -czf "$release.tar.gz" "$release"
rm -rf "$release"