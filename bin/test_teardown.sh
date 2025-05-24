#!/usr/bin/env sh

if ! command -v multipass &> /dev/null
then
    echo "multipass could not be found"
    exit
fi

multipass delete node1 node2 --purge

multipass list