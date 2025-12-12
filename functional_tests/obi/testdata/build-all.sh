#!/bin/bash

# Script to build and tag all distributed tracing test applications locally
# Images are tagged for KIND cluster loading (not pushed to registry)

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$SCRIPT_DIR"

IMAGES=(
  "go:go"
  "java:java"
  "nodejs:nodejs"
  "dotnet:dotnet"
  "python:python"
  "ruby:ruby"
  "cpp:cpp"
  "rust:rust"
)

echo "Building distributed tracing test applications..."
echo "=================================================="

for image in "${IMAGES[@]}"; do
  IFS=':' read -r dir lang <<< "$image"
  
  echo ""
  echo "Building $lang application..."
  
  if [ ! -d "$dir" ]; then
    echo "Error: Directory $dir not found"
    exit 1
  fi
  
  image_name="distributed-trace-test-${lang}:latest"
  
  docker build -t "$image_name" "$dir"
  
  if [ $? -eq 0 ]; then
    echo "✓ Successfully built $image_name"
    docker images "$image_name" --no-trunc
  else
    echo "✗ Failed to build $image_name"
    exit 1
  fi
done

echo ""
echo "=================================================="
echo "All images built successfully!"
echo ""
echo "Built images:"
docker images distributed-trace-test-* --no-trunc
