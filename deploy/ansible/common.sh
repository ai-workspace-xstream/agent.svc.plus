#!/bin/bash

resolve_inventory_path() {
    local inventory_file="$1"

    if [[ "$inventory_file" = /* ]]; then
        printf '%s\n' "$inventory_file"
    else
        printf '%s\n' "$SCRIPT_DIR/$inventory_file"
    fi
}

inventory_first_host_line() {
    local inventory_file="$1"

    awk '
        /^\[agent_svc_plus\]/ { in_group=1; next }
        /^\[/ { in_group=0 }
        in_group && $0 !~ /^[[:space:]]*#/ && NF { print; exit }
    ' "$inventory_file"
}

inventory_first_host_alias() {
    local inventory_file="$1"
    local host_line

    host_line="$(inventory_first_host_line "$inventory_file")" || return 1
    [ -n "$host_line" ] || return 1

    printf '%s\n' "$host_line" | awk '{ print $1 }'
}

inventory_first_host_field() {
    local inventory_file="$1"
    local key="$2"
    local host_line

    host_line="$(inventory_first_host_line "$inventory_file")" || return 1
    [ -n "$host_line" ] || return 1

    printf '%s\n' "$host_line" | awk -v key="$key" '
        {
            for (i = 1; i <= NF; i++) {
                split($i, pair, "=")
                if (pair[1] == key) {
                    print substr($i, length(key) + 2)
                    exit
                }
            }
        }
    '
}

inventory_all_var() {
    local inventory_file="$1"
    local key="$2"

    awk -v key="$key" '
        /^\[all:vars\]/ { in_group=1; next }
        /^\[/ { in_group=0 }
        in_group && $0 !~ /^[[:space:]]*#/ && index($0, key "=") == 1 {
            print substr($0, length(key) + 2)
            exit
        }
    ' "$inventory_file"
}

inventory_target_host() {
    local inventory_file="$1"
    local host_alias
    local ansible_host

    host_alias="$(inventory_first_host_alias "$inventory_file")" || return 1
    ansible_host="$(inventory_first_host_field "$inventory_file" "ansible_host" || true)"

    printf '%s\n' "${ansible_host:-$host_alias}"
}

inventory_target_user() {
    local inventory_file="$1"
    local ansible_user

    ansible_user="$(inventory_first_host_field "$inventory_file" "ansible_user" || true)"
    if [ -z "$ansible_user" ]; then
        ansible_user="$(inventory_all_var "$inventory_file" "ansible_user" || true)"
    fi

    printf '%s\n' "${ansible_user:-root}"
}

inventory_target_port() {
    local inventory_file="$1"
    local ansible_port

    ansible_port="$(inventory_first_host_field "$inventory_file" "ansible_port" || true)"
    if [ -z "$ansible_port" ]; then
        ansible_port="$(inventory_all_var "$inventory_file" "ansible_port" || true)"
    fi

    printf '%s\n' "${ansible_port:-22}"
}

vars_agent_id() {
    local vars_file="$1"

    awk '
        /^[[:space:]]*agent_id:/ {
            value = $0
            sub(/^[[:space:]]*agent_id:[[:space:]]*/, "", value)
            gsub(/"/, "", value)
            print value
            exit
        }
    ' "$vars_file"
}

vars_cloudflare_record_ip() {
    local vars_file="$1"

    awk '
        /cloudflare_dns_records:/ { in_records=1; next }
        in_records && /^[[:space:]]*content:/ {
            gsub(/"/, "", $2)
            print $2
            exit
        }
    ' "$vars_file"
}
