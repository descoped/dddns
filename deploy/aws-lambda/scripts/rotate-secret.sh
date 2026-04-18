#!/usr/bin/env bash
# rotate-secret.sh — generate a new dyndns shared secret and store it in SSM.
#
# Usage:
#   rotate-secret.sh                    # reads SSM param name + region from ../tofu outputs
#   rotate-secret.sh -n /some/name      # explicit SSM parameter name
#   rotate-secret.sh -n /name -r us-east-1
#
# What it does:
#   1. Generates a 32-byte (256-bit) hex secret via openssl.
#   2. Calls `aws ssm put-parameter --overwrite` to replace the
#      existing SecureString value.
#   3. Prints the new secret in a framed block so it's easy to copy
#      into UniFi UI → Internet → Dynamic DNS → Password.
#
# The secret is printed to stdout exactly once. It is NOT written
# to any file, logged anywhere, or sent over the network except
# to AWS SSM. If you miss the copy, run the script again.
#
# Requires: bash 4+, openssl, AWS CLI v2 authenticated to the
# target account with ssm:PutParameter on the parameter ARN.

set -euo pipefail

PARAM_NAME=""
REGION=""

usage() {
    cat <<EOF
Usage: $(basename "$0") [-n SSM_PARAM_NAME] [-r AWS_REGION]

Options:
  -n NAME    SSM parameter name (e.g. /dddns/shared_secret).
             Defaults to the 'ssm_parameter_name' output of the
             ../tofu module if available.
  -r REGION  AWS region. Defaults to the 'aws_region' variable of
             the ../tofu module, or AWS_REGION env var, or us-east-1.
  -h         Show this help.
EOF
}

while getopts "n:r:h" opt; do
    case "${opt}" in
        n) PARAM_NAME="${OPTARG}" ;;
        r) REGION="${OPTARG}" ;;
        h) usage; exit 0 ;;
        *) usage; exit 2 ;;
    esac
done

# Try to read the parameter name + region from tofu outputs if not
# supplied on the command line. This keeps the common case frictionless
# right after `tofu apply` while allowing full override.
TOFU_DIR="$(cd "$(dirname "$0")/../tofu" && pwd)"
if [[ -z "${PARAM_NAME}" ]]; then
    if tofu -chdir="${TOFU_DIR}" output -raw ssm_parameter_name 2>/dev/null > /tmp/.dddns-param-name; then
        PARAM_NAME="$(cat /tmp/.dddns-param-name)"
        rm -f /tmp/.dddns-param-name
    fi
fi

if [[ -z "${PARAM_NAME}" ]]; then
    echo "ERROR: SSM parameter name not provided and tofu output unavailable." >&2
    echo "       Pass -n /path/to/parameter explicitly, or run from deploy/aws-lambda/" >&2
    exit 2
fi

# Region precedence: -r flag > AWS_REGION env > tofu aws_region > us-east-1.
if [[ -z "${REGION}" && -n "${AWS_REGION:-}" ]]; then
    REGION="${AWS_REGION}"
fi
if [[ -z "${REGION}" ]]; then
    if REGION="$(tofu -chdir="${TOFU_DIR}" output -raw aws_region 2>/dev/null)"; then
        :
    else
        REGION=""
    fi
fi
REGION="${REGION:-us-east-1}"

# Pre-flight: confirm AWS creds are loaded.
if ! aws sts get-caller-identity --region "${REGION}" >/dev/null 2>&1; then
    echo "ERROR: AWS CLI is not authenticated (aws sts get-caller-identity failed)." >&2
    echo "       Configure credentials (aws configure / env vars / SSO) before running." >&2
    exit 1
fi

SECRET="$(openssl rand -hex 32)"

aws ssm put-parameter \
    --region "${REGION}" \
    --name "${PARAM_NAME}" \
    --type SecureString \
    --value "${SECRET}" \
    --overwrite \
    --description "dddns dyndns-v2 shared secret (rotated $(date -u +%Y-%m-%dT%H:%M:%SZ))" \
    --output text >/dev/null

cat <<EOF

┌──────────────────────────────────────────────────────────────────────────┐
│                                                                          │
│  New shared secret stored in SSM parameter: ${PARAM_NAME}
│                                                                          │
│  Paste this value into UniFi UI → Internet → Dynamic DNS → Password:     │
│                                                                          │
│      ${SECRET}
│                                                                          │
│  The Lambda picks up the rotated value within 60s (secret cache TTL).    │
│  The old secret continues to work during that overlap window.            │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘

EOF
