#!/bin/sh
#
# Use this script to run your program LOCALLY.

set -e # Exit early if any commands fail

(
  cd "$(dirname "$0")" # Ensure compile steps are run within the repository directory
  go build -o build/gocode src/*.go
)

exec build/gocode "$@"
