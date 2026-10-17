#!/bin/sh
# Renders the PNG icons from kiln.svg. The PNGs are committed, so only
# a change to the icon needs this (brew install librsvg).
set -eu
cd "$(dirname "$0")"
for n in 192 512; do
  rsvg-convert -w $n -h $n kiln.svg -o icon-$n.png
done
rsvg-convert -w 180 -h 180 kiln.svg -o apple-touch-icon.png
