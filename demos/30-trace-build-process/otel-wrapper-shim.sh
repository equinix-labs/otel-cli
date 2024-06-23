#!/bin/bash

# Build Process Tracing Example
#
# It's possible to instrument complex build processes in nodejs (npm/yarn/pnpm) and
# C (make/cmake/ninja) by injecting instrumented versions of some key commands into $PATH.
#
# This example is a folder with a shim script in it, and a bunch of fake tools that are just
# symlinks to the shim. It needs to be a folder so that it can be cleanly injected into $PATH.
#
# To see this working, start `otel-desktop-viewer` as discribed in the main README.md, and then:
# (
#   cd .. && \
#   ([ -d mermaid ] || git clone https://github.com/mermaid-js/mermaid) && \
#   cd mermaid/ && \
#   ../otel-cli/demos/30-trace-build-process/pnpm install
# )
#
# For more complex build processes, I have found jaeger to be quite good, because it also includes
# a black "critical path line". See https://www.jaegertracing.io/docs/1.54/getting-started/ for
# their all-in-one docker conainer setup instructions. Otherwise, everything else sis the same as
# when using otel-desktop-viewer

set -euo pipefail

export OTEL_EXPORTER_OTLP_ENDPOINT="${OTEL_EXPORTER_OTLP_ENDPOINT:-localhost:4317}"

TOOL_NAME="$(basename $0)"
LOCATION_IN_PATH="$(dirname $0)"
HERE="$(dirname $(readlink -f $0))"
# This is just a guess, based on what's left after we've removed ourselves and any symlinks we know about from the results.
# If we're symlinked into $PATH in more than once place then we could still end up in loops.
# If this becomes a problem, we could instead iterate over each location and call readlink,
# and drop anything that resolves to $HERE.
ORIGINAL_TOOL_PATH="$(which -a "$TOOL_NAME" | grep -Ev "($HERE|$LOCATION_IN_PATH)" | head -n1)"

# Put this dir first in $PATH so that nested calls out to bash and pnpm are instrumented
# (tools like npm have a habit of messing with $PATH to put themselves first in subshells).
# This will probably get quite long if there is a lot of recursion.
# If this causes problems, we could prune ouselves out before inserting ourself at the head.
export PATH="$HERE:$PATH"

otel-cli exec --service "$TOOL_NAME" --name "$*" -- $ORIGINAL_TOOL_PATH "$@"
