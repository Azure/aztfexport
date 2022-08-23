#!/bin/bash
set -e

version=${1:?"version not specified"}

sed -i 's/mirrorlist/#mirrorlist/g' /etc/yum.repos.d/CentOS-*
sed -i 's|#baseurl=http://mirror.centos.org|baseurl=http://vault.centos.org|g' /etc/yum.repos.d/CentOS-*
rpm --import https://packages.microsoft.com/keys/microsoft.asc
dnf install -y https://packages.microsoft.com/config/centos/8/packages-microsoft-prod.rpm

# See: https://access.redhat.com/solutions/2779441
dnf check-update || [[ $? == 100 ]]  


total=20
count=1
while ((count <= total)); do
    dnf install -y aztfy && break
    echo "Retry ($count/$total)"
    sleep 1m
    ((count++))
done
(( count <= total ))

grep $version <(aztfy -v)
