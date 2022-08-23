#!/bin/bash
set -e

version=${1:?"version not specified"}

apt-get update
apt-get install -y curl software-properties-common gpg
curl -sSL https://packages.microsoft.com/keys/microsoft.asc > /etc/apt/trusted.gpg.d/microsoft.asc
apt-add-repository https://packages.microsoft.com/ubuntu/22.04/prod
apt-get update

total=60
count=1
while ((count <= total)); do
    apt-get install -y aztfy && break
    echo "Retry ($count/$total)"
    sleep 1m
    ((count++))
done

grep $version <(aztfy -v)
