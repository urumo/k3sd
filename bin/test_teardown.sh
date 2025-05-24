#!/usr/bin/env sh

if ! command -v multipass &> /dev/null
then
  echo "Error: multipass could not be found. Please install multipass first."
  exit 1
fi

multipass delete node1 node2 --purge

multipass list