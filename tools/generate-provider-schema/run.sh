#!/bin/bash

MYDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MYNAME="$(basename "${BASH_SOURCE[0]}")"
ROOTDIR="$MYDIR/../.."

die() {
    echo "$@" >&2
    exit 1
}

usage() {
    cat << EOF
Usage: ./run.sh [options] 

Options:
    -h|--help           show this message

Arguments:
    provider_dir        The path to the AzureRM provider repo
    provider_version    The version of the AzureRM provider (e.g. 2.72.0)
EOF
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
    expect_n_arg=2
    [[ $# = "$expect_n_arg" ]] || die "wrong arguments (expected: $expect_n_arg, got: $#)"

    provider_dir=$1
    provider_version=$2

    [[ -d $provider_dir ]] || die "no such directory: $provider_dir"

    cp -r "$MYDIR/generate-provider-schema" "$provider_dir/internal/tools"
    pushd $provider_dir > /dev/null
    git checkout "v$provider_version" || die "failed to checkout provider version $provider_version"
    go mod tidy || die "failed to run go mod tidy"
    go mod vendor || die "failed to run go mod vendor"
    out=$(go run "$provider_dir/internal/tools/generate-provider-schema/main.go") || die "failed to generate provider schema"
    cat << EOF > "$ROOTDIR/schema/provider_gen.go"
package schema

import (
	"encoding/json"
	"fmt"
	"os"
)

var ProviderVersion = "$provider_version"

var ProviderSchemaInfo ProviderSchema

func init() {
    b := []byte(\`$out\`)
	if err := json.Unmarshal(b, &ProviderSchemaInfo); err != nil {
		fmt.Fprintf(os.Stderr, "unmarshalling the provider schema: %s", err)
		os.Exit(1)
	}
}
EOF
    popd > /dev/null
}

main "$@"
