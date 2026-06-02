#!/bin/bash
set -e
cd "$(dirname "$0")/.."
go run ./cmd/job/trivial/publish-event --config config/publish-event-local.yaml
