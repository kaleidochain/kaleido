#!/bin/sh

set -e

if [ ! -f "build/env.sh" ]; then
    echo "$0 must be run from the root of the repository."
    exit 2
fi

# Create fake Go workspace if it doesn't exist yet.
workspace="$PWD/build/_workspace"
root="$PWD"
dir="$workspace/src/github.com/kaleidochain"
if [ ! -L "$dir/kaleido" ]; then
    mkdir -p "$dir"
    cd "$dir"
    ln -s ../../../../../. kaleido
    cd "$root"
fi

# Set up the environment to use the workspace.
GOPATH="$workspace"
export GOPATH

# Run the command inside the workspace.
cd "$dir/kaleido"
PWD="$dir/kaleido"

# Launch the arguments with the configured environment.
exec "$@"
