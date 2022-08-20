#!/bin/bash
set -e

version=${1:?"version not specified"}

rpm --import https://packages.microsoft.com/keys/microsoft.asc
dnf install -y https://packages.microsoft.com/config/fedora/36/packages-microsoft-prod.rpm
dnf check-update
dnf install -y aztfy
grep $version <(aztfy -v)
