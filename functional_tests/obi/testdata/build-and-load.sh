#!/bin/bash

# Script to build and/or load distributed tracing test images for KIND clusters

set -e

PROGNAME="$(basename "$0")"
readonly PROGNAME
PROGDIR="$(readlink -m "$(dirname "$0")")"
readonly PROGDIR
readonly ARGS="$*"

# Images to build/load.
readonly IMAGES=(
  "go:go"
  "java:java"
  "nodejs:nodejs"
  "dotnet:dotnet"
  "python:python"
  "ruby:ruby"
  "cpp:cpp"
  "rust:rust"
)

usage() {
    cat <<- EOF
	usage: $PROGNAME [options]

	Program builds and/or loads distributed tracing test images into KIND cluster.

	OPTIONS:
	   -b --build-only          only build images, do not load into KIND
	   -l --load-only           only load images into KIND, do not build
	   -c --cluster             KIND cluster name (default: kind)
	   -v --verbose             verbose output (can be specified multiple times)
	   -h --help                show this help

	Examples:
	   Build and load images:
	   $PROGNAME

	   Only build images:
	   $PROGNAME --build-only

	   Only load images:
	   $PROGNAME --load-only

	   Load into specific cluster with verbose output:
	   $PROGNAME -l -c my-cluster -v
	EOF
}

log_info() {
    local message="$1"
    echo "$message"
}

log_verbose() {
    local level=$1
    shift
    local message="$*"
    
    [[ $VERBOSE -ge $level ]] && echo "$message"
}

log_error() {
    local message="$1"
    echo "Error: $message" >&2
}

is_empty() {
    local var=$1
    [[ -z $var ]]
}

is_dir() {
    local dir=$1
    [[ -d $dir ]]
}

is_build_enabled() {
    [[ $DO_BUILD -eq 1 ]]
}

is_load_enabled() {
    [[ $DO_LOAD -eq 1 ]]
}

check_docker_available() {
    if ! command -v docker &> /dev/null; then
        log_error "docker command not found"
        return 1
    fi
    return 0
}

check_kind_available() {
    if ! command -v kind &> /dev/null; then
        log_error "kind command not found"
        return 1
    fi
    return 0
}

get_image_name() {
    local lang=$1
    echo "distributed-trace-test-${lang}:latest"
}

build_image() {
    local dir=$1
    local lang=$2
    local image_name
    image_name=$(get_image_name "$lang")
    
    log_verbose 1 "Building $lang application from $dir..."
    
    if ! is_dir "$dir"; then
        log_error "Directory $dir not found"
        return 1
    fi
    
    local build_result
    if [[ $VERBOSE -ge 2 ]]; then
        if docker build -t "$image_name" "$dir"; then
            build_result=0
        else
            build_result=1
        fi
    else
        if docker build -t "$image_name" "$dir" > /dev/null 2>&1; then
            build_result=0
        else
            build_result=1
        fi
    fi
    
    if [[ $build_result -eq 0 ]]; then
        log_info "✓ Successfully built $image_name"
        log_verbose 2 "$(docker images "$image_name" --no-trunc)"
        return 0
    else
        log_error "Failed to build $image_name"
        return 1
    fi
}

build_all_images() {
    log_info "Building distributed tracing test applications..."
    log_info "=================================================="
    
    local image
    for image in "${IMAGES[@]}"; do
        IFS=':' read -r dir lang <<<"$image"
        
        log_verbose 1 ""
        
        local full_path="$PROGDIR/$dir"
        build_image "$full_path" "$lang" || return 1
    done
    
    log_info ""
    log_info "=================================================="
    log_info "All images built successfully!"
    
    if [[ $VERBOSE -ge 1 ]]; then
        log_info ""
        log_info "Built images:"
        docker images distributed-trace-test-* --no-trunc
    fi
    
    return 0
}

load_image() {
    local image=$1
    local cluster=$2
    
    log_verbose 1 "Loading ${image}..."
    
    local load_result
    if [[ $VERBOSE -ge 2 ]]; then
        if kind load docker-image "${image}" --name "${cluster}"; then
            load_result=0
        else
            load_result=1
        fi
    else
        if kind load docker-image "${image}" --name "${cluster}" > /dev/null 2>&1; then
            load_result=0
        else
            load_result=1
        fi
    fi
    
    if [[ $load_result -eq 0 ]]; then
        log_verbose 1 "✓ Successfully loaded $image"
        return 0
    else
        log_error "Failed to load $image"
        return 1
    fi
}

load_all_images() {
    log_info "Loading distributed tracing test images into KIND cluster '${CLUSTER_NAME}'..."
    
    local image
    for image in "${IMAGES[@]}"; do
        IFS=':' read -r dir lang <<<"$image"
        local image_name
        image_name=$(get_image_name "$lang")
        
        load_image "$image_name" "$CLUSTER_NAME" || return 1
    done
    
    log_info ""
    log_info "All images loaded successfully into KIND cluster '${CLUSTER_NAME}'!"
    
    return 0
}

cmdline() {
    local arg=
    local args=
    local verbose_count=0
    local do_build=1
    local do_load=1
    local cluster_name="${KIND_CLUSTER_NAME:-kind}"
    
    for arg in "$@"; do
        local delim=""
        case "$arg" in
            --build-only)
                args="${args}-b "
                ;;
            --load-only)
                args="${args}-l "
                ;;
            --cluster)
                args="${args}-c "
                ;;
            --verbose)
                args="${args}-v "
                ;;
            --help)
                args="${args}-h "
                ;;
            *)
                [[ "${arg:0:1}" == "-" ]] || delim="\""
                args="${args}${delim}${arg}${delim} "
                ;;
        esac
    done
    
    eval set -- "$args"
    
    while getopts "blc:vh" OPTION; do
        case $OPTION in
            b)
                do_build=1
                do_load=0
                ;;
            l)
                do_build=0
                do_load=1
                ;;
            c)
                cluster_name=$OPTARG
                ;;
            v)
                verbose_count=$((verbose_count + 1))
                ;;
            h)
                usage
                exit 0
                ;;
            *)
                usage
                exit 1
                ;;
        esac
    done
    
    readonly VERBOSE=$verbose_count
    readonly DO_BUILD=$do_build
    readonly DO_LOAD=$do_load
    readonly CLUSTER_NAME=$cluster_name
    
    return 0
}

validate_inputs() {
    if ! is_build_enabled && ! is_load_enabled; then
        log_error "Cannot specify both --build-only and --load-only"
        usage
        return 1
    fi
    
    if is_load_enabled; then
        if is_empty "$CLUSTER_NAME"; then
            log_error "Cluster name cannot be empty"
            return 1
        fi
    fi
    
    return 0
}

check_prerequisites() {
    if is_build_enabled; then
        check_docker_available || return 1
    fi
    
    if is_load_enabled; then
        check_kind_available || return 1
    fi
    
    return 0
}

main() {
    cmdline "$ARGS"
    
    validate_inputs || exit 1
    
    check_prerequisites || exit 1
    
    if is_build_enabled; then
        build_all_images || exit 1
    fi
    
    if is_load_enabled; then
        if is_build_enabled; then
            log_info ""
        fi
        load_all_images || exit 1
    fi
    
    return 0
}

main
