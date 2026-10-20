#!/bin/sh
# Renders the PNG icons from kiln.svg. The PNGs are committed, so only
# a change to the icon needs this (brew install librsvg). It renders the
# desktop app's too, which wails3 makes its .icns and .ico from.
#
# The maskable icons are for Android, which crops an icon to its own
# shape: only a centered circle 80% across is sure to show. They put the
# art at 67% on a fill of its brown bands, so the bands run into the
# fill, and the chevron field (full width, the middle 65% of the
# height) just fits in that circle: 0.67 × √(1 + 0.65²) ≈ 0.8.
set -eu
cd "$(dirname "$0")"
brown='#72421d' # the bands' rgb(114,66,29) in kiln.svg
maskable=$(mktemp "${TMPDIR:-/tmp}/kiln-maskable.XXXXXX")
trap 'rm -f "$maskable"' EXIT
# The art goes in inline (rsvg-convert won't load an <image> from a
# file), in a nested <svg> that shows only the chevron field (y 175 to
# 825 in kiln.svg): the fill is the bands, and clipping the art's own
# bands would leave a seam of chevron at their edges.
{
  printf '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1000 1000">\n'
  printf '<rect width="1000" height="1000" fill="%s"/>\n' "$brown"
  printf '<svg x="165" y="282.25" width="670" height="435.5" viewBox="0 175 1000 650">\n'
  sed -n '/<svg /,/<\/svg>/p' kiln.svg | sed '1d;$d'
  printf '</svg>\n</svg>\n'
} > "$maskable"
for n in 192 512; do
  rsvg-convert -w $n -h $n kiln.svg -o icon-$n.png
  rsvg-convert -w $n -h $n "$maskable" -o icon-maskable-$n.png
done
rsvg-convert -w 180 -h 180 kiln.svg -o apple-touch-icon.png
rsvg-convert -w 1024 -h 1024 kiln.svg -o ../../../desktop/build/appicon.png
