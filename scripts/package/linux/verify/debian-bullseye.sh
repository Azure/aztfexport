#!/bin/bash
set -e

version=${1:?"version not specified"}

apt-get update
apt-get install -y curl software-properties-common gpg
curl -sSL https://packages.microsoft.com/keys/microsoft.asc | apt-key add -
apt-add-repository https://packages.microsoft.com/debian/11/prod

total=60
count=1
while ((count <= total)); do
    echo "Try ($count/$total)"
    apt-get update
    apt-get install -y aztfexport && grep $version <(aztfexport -v) && break
    sleep 1m
    ((count++))
done
(( count <= total ))
