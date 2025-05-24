#!/usr/bin/env sh

if ! command -v multipass &> /dev/null
then
  echo "Error: multipass could not be found. Please install multipass first."
  exit 1
fi

multipass launch --name node1 --cpus 2 --mem 2G --disk 20G
multipass launch --name node2 --cpus 2 --mem 2G --disk 20G

PASS=${MPS_PASSWORD:-password123}
multipass exec node1 -- sudo bash -c "echo ubuntu:${PASS} | chpasswd"
multipass exec node2 -- sudo bash -c "echo ubuntu:${PASS} | chpasswd"

multipass info --all | grep "IPv4"

