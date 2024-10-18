#!/bin/bash
#
# Copyright 2022 Red Hat, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# ========================================================================
#
# Use this script to install the Authorino Operator and dependencies
# to the current kubectl context, without having to clone the repo
# or depending on Operator Lifecycle Manager (OLM).
#
# E.g.:
#   curl -sL https://raw.githubusercontent.com/Kuadrant/authorino-operator/main/utils/install.sh | bash -s -- [options]
#

VERSION='latest'
CERT_MANAGER_VERSION="1.12.1"
NO_DEPS=''
DRY_RUN=''

print_usage() {
  cat <<EOF
Installs the Authorino Operator and dependencies to the current kubectl context.

Usage:

  install.sh [options]

Options:

  -v | --version    The version of the Authorino Operator to install. Defaults to 'latest'.
       --git-ref    The Git reference of the Authorino Operator to install.
                    Provide it in the form of a branch, tag or commit hash, alternatively to providing a version.
                    If omitted, the Git reference will be inferred from the version.
       --no-deps    Do not install dependencies.
       --dry-run    Print the commands that would be executed, but do not execute them.
  -h | --help       Print this help message.
EOF
}

while test $# -gt 0; do
  case "$1" in
    -v|--version)
      shift
      if test $# -gt 0; then
        VERSION=$1
      else
        echo "missing version argument"
        exit 1
      fi
      shift
      ;;
    --git-ref)
      shift
      if test $# -gt 0; then
        GIT_REF=$1
      else
        echo "missing git-ref argument"
        exit 1
      fi
      shift
      ;;
    --no-deps)
      shift
      NO_DEPS='true'
      ;;
    --dry-run)
      shift
      DRY_RUN='true'
      ;;
    -h|--help)
      print_usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1"
      print_usage
      exit 1
      ;;
  esac
done

set -o pipefail -e

steps=()
cmds=()

step() {
  local descr=$1
  shift
  append_step "$descr"
  append_cmd "$@"
}

append_step() {
  steps+=("$*")
}

append_cmd() {
  cmds+=("$*")
}

run() {
  local steps_curr=0
  local steps_total="${#steps[@]}"
  for i in "${!steps[@]}"; do
    steps_curr=$((steps_curr + 1))
    echo -e "[\033[0;34m${steps_curr}/${steps_total}\033[0m] ${steps[$i]}"
    eval "${cmds[$i]}"
  done
}

cmd() {
  if [[ -z $DRY_RUN ]]; then
    eval "$@"
  else
    echo "$@"
  fi
}

# Define your steps below this point.
# Wrap individual commands in a `cmd` function call to skip and print them to STDOUT in dry-run mode.
#
# E.g.:
#   step "Installing foo..." cmd arg1 arg2

install_cert_manager() {
  cmd kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v${CERT_MANAGER_VERSION}/cert-manager.yaml
  cmd kubectl -n cert-manager wait --timeout=300s --for=condition=Available deployments --all
}

install_operator() {
  cmd kubectl apply -f https://raw.githubusercontent.com/Kuadrant/authorino-operator/${GIT_REF}/config/deploy/manifests.yaml
}

# if [[ -z $NO_DEPS ]]; then
#   step "Installing \033[0;32mcert-manager\033[0m \033[0;37m(version: ${CERT_MANAGER_VERSION})\033[0m to the current kubectl context..." \
#        install_cert_manager
# fi

version_info="git-ref: $GIT_REF"
if [[ -z $GIT_REF ]]; then
  version_info="version: $VERSION"
  if [[ $VERSION == "latest" ]]; then
    GIT_REF="main"
  else
    if [[ $VERSION =~ ^[0-9]+\.[0-9]+\.[0-9]+(-.+)?$ ]]; then
      GIT_REF="v${VERSION}"
    else
      GIT_REF="${VERSION}"
    fi
  fi
fi

step "Installing \033[0;32mAuthorino Operator\033[0m \033[0;37m(${version_info})\033[0m to the current kubectl context..." \
     install_operator

# Runs all the steps in order
run
