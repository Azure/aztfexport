#!/bin/bash
set -e

version=${1:?"version not specified"}

apt-get update
apt-get install -y curl software-properties-common gpg
curl -sSL https://packages.microsoft.com/keys/microsoft.asc | apt-key add -
apt-add-repository https://packages.microsoft.com/ubuntu/18.04/multiarch/prod
apt-get update
apt-get install -y aztfy
grep $version <(aztfy -v)
