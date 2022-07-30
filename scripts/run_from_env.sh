#!/usr/bin/env bash

set -eu

export PORT=8000
export SB_FEDERATES="https://spring83.kindrobot.ca,https://0l0.lol,https://bogbody.biz,https://spring83.mozz.us/"
export SB_FQDN="localhost:8000"
export SB_ADMIN_BOARD="bf71bb0d73bc3b0edfd0bd750f9e191c476773b3660d9ba86d658b49083e0623"

go run ./cmd/springboard serve
