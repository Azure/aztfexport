#!/bin/bash

set -e

usage() {
    cat << EOF
Usage: ./${MYNAME} [options] version sourcedir outdir

Options:
    -h|--help           show this message

Arguments:
    version             Version of aztfy, e.g. v0.1.0.
    sourcedir           The directory contains arch named subfolders, which in turn include the "aztfy" binary for that arch 
                        (Sub-folder includes: 386, amd64, arm, arm64)
    outdir              The output directory, that contains the packages.
EOF
}

die() {
    echo "$@" >&2
    exit 1
}

main() {
    while :; do
        case $1 in
            -h|--help)
                usage
                exit 1
                ;;
            --)
                shift
                break
                ;;
            *)
                break
                ;;
        esac
        shift
    done

    local expect_n_arg
    expect_n_arg=3
    [[ $# = "$expect_n_arg" ]] || die "wrong arguments (expected: $expect_n_arg, got: $#)"

    version=$1
    [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || die 'version should be of form "vx.y.z"'
    version=${version:1}
    sourcedir=$2
    outdir=$3

    cat << EOF > .fpm
--name aztfy
--license MPL-2.0
--version $version
--description "A tool to bring existing Azure resources under Terraform's management"
--url "https://github.com/Azure/aztfy"
--maintainer "magodo <wztdyl@sina.com>"
EOF
    pkg_debian $version $sourcedir $outdir
    pkg_rpm $version $sourcedir $outdir
}

pkg_debian() {
    version=$1
    sourcedir=$2
    outdir=$3
    mkdir -p $outdir/debian
    declare -A arch_map=( [386]=i386 [amd64]=amd64 [arm]=armhf [arm64]=arm64 )
    for arch in 386 amd64 arm arm64; do
        fpm -a ${arch_map[$arch]} -s dir -t deb -p $outdir/debian/aztfy-$version-1-${arch_map[$arch]}.deb $sourcedir/$arch/aztfy=/usr/bin/aztfy
    done
}

pkg_rpm() {
    version=$1
    sourcedir=$2
    outdir=$3
    mkdir -p $outdir/rpm
    declare -A arch_map=( [386]=i686 [amd64]=x86_64 [arm]=armv7hl [arm64]=aarch64 )
    for arch in 386 amd64 arm arm64; do
        fpm -a ${arch_map[$arch]} -s dir -t rpm -p $outdir/rpm/aztfy-$version-1-${arch_map[$arch]}.rpm $sourcedir/$arch/aztfy=/usr/bin/aztfy
    done
}

main "$@"
