#!/bin/bash
set -e

apt-get update
apt-get install -y curl software-properties-common gpg
curl -sSL https://packages.microsoft.com/keys/microsoft.asc | apt-key add -
apt-add-repository https://packages.microsoft.com/repos/microsoft-ubuntu-bionic-multiarch-prod
apt-get update
apt-get install -y aztfy
aztfy -v
