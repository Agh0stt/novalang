#!/bin/bash
# Build script for the Nova compiler
# Run: bash build.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

echo "=== Building Nova Compiler ==="
go build -o nova ./cmd/nova/

echo "=== Build successful: ./nova ==="
echo ""
echo "Usage examples:"
echo "  ./nova examples/hello.nv          # compile to binary"
echo "  ./nova -S examples/hello.nv       # emit assembly only"
echo "  ./nova -tokens examples/hello.nv  # dump token stream"
echo "  ./nova -ast examples/hello.nv     # dump AST"
echo ""
echo "To install system-wide:"
echo "  cp nova /usr/local/bin/nova"
