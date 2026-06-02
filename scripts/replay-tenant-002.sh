#!/bin/bash
set -e
cd "$(dirname "$0")/.."
go run ./cmd/job/trivial/dead-letter-replay --config config/dead-letter-replay-local.yaml
