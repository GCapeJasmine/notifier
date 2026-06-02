#!/bin/bash
set -e
cd "$(dirname "$0")/.."
go run ./cmd/job/trivial/mock-partner/
