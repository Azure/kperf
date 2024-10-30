#!/usr/bin/env bash
#
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.

set -euo pipefail

result_file=/data/${POD_NAMESPACE}-${POD_NAME}-${POD_UID}.json

/kperf -v=2 runner run --config=/config/load_profile.yaml \
		--user-agent=${POD_NAME} \
    --result=${result_file} \
    --raw-data

while true; do
  set +e
  http_code=$(curl -s -o /dev/null -w "%{http_code}" -XPOST -d "@${result_file}" ${TARGET_URL} || "50X")
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
