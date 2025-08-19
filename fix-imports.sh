#!/bin/bash

# Fix all imports from github.com/mExOms/oms to github.com/mExOms
find . -name "*.go" -type f -exec sed -i 's|github.com/mExOms/oms|github.com/mExOms|g' {} \;

echo "Fixed all imports"