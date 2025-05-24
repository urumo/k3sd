#!/usr/bin/env sh

if ! command -v multipass &> /dev/null
then
    echo "multipass could not be found"
    exit
fi

multipass launch --name node1 --cpus 2 --mem 2G --disk 20G
multipass launch --name node2 --cpus 2 --mem 2G --disk 20G

multipass exec node1 -- sudo bash -c 'echo "ubuntu:password123" | chpasswd'
multipass exec node2 -- sudo bash -c 'echo "ubuntu:password123" | chpasswd'

multipass info --all | grep "IPv4"

