#!/usr/bin/env sh

if ! command -v multipass &> /dev/null
then
  echo "Error: multipass could not be found. Please install multipass first."
  exit 1
fi

multipass launch --name node1 --cpus 2 --memory 2G --disk 20G
multipass launch --name node2 --cpus 2 --memory 2G --disk 20G

PASS=${MPS_PASSWORD:-password123}
multipass exec node1 -- sudo bash -c "echo ubuntu:${PASS} | sudo chpasswd"
multipass exec node2 -- sudo bash -c "echo ubuntu:${PASS} | sudo chpasswd"

for key in ~/.ssh/*.pub; do
  multipass exec node1 -- bash -c "echo '$(cat "$key")' >> ~/.ssh/authorized_keys"
  multipass exec node2 -- bash -c "echo '$(cat "$key")' >> ~/.ssh/authorized_keys"
done

multipass info | grep "IPv4"

