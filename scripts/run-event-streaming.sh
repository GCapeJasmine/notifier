#!/bin/bash
set -e
cd "$(dirname "$0")/.."
go run ./cmd/event-streaming --config config/notify-webhook-local.yaml
