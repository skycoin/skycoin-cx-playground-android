#!/bin/sh
# R13: one declaration per nnn-name.cx file, numbered in import order.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
"${PORTLAYOUT:?set PORTLAYOUT}" "$here/../../port"
