#!/usr/bin/env bash
#
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
#
# Runner script for replay mode. Used by runner pods to execute replay tests.
# Expects the following environment variables:
#   - POD_NAME: Name of the pod
#   - POD_NAMESPACE: Namespace of the pod
#   - POD_UID: UID of the pod
#   - TARGET_URL: URL to upload results to
#   - RUNNER_VERBOSITY: Log verbosity level
#   - REPLAY_PROFILE_SOURCE: Path or URL to the replay profile

set -euo pipefail

result_file=/data/${POD_NAMESPACE}-${POD_NAME}-${POD_UID}.json

/kperf -v="${RUNNER_VERBOSITY}" runner replay \
    --config="${REPLAY_PROFILE_SOURCE}" \
    --result="${result_file}" \
    --raw-data

while true; do
  set +e
  http_code=$(curl -s -o /dev/null -w "%{http_code}" -XPOST --data-binary "@${result_file}" "${TARGET_URL}" || echo "50X")
  set -e

  case $http_code in
    201)
      echo "Uploaded it"
      exit 0
      ;;
    409)
      echo "Has been uploaded, skip"
      exit 0;
      ;;
    404)
      echo "Leaking pod? skip"
      exit 1;
      ;;
    *)
      echo "Need to retry after received http code ${http_code} (or failed to connect)"
      sleep 5s
      ;;
  esac
done
