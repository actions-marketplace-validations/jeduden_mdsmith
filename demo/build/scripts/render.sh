#!/bin/sh
# User-defined render recipe — count words in the input.
awk '{c+=NF} END{printf "words: %d\n", c}' "$1" > "$2"
