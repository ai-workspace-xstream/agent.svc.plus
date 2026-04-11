#!/bin/bash

# install-hooks.sh - Deploy Git hooks to the current repository

HOOKS_DIR=$(git rev-parse --git-path hooks)
SOURCE_DIR="scripts/hooks"

echo "Installing Git hooks to $HOOKS_DIR..."

install_hook() {
    local hook_name=$1
    if [ -f "$SOURCE_DIR/$hook_name" ]; then
        cp "$SOURCE_DIR/$hook_name" "$HOOKS_DIR/$hook_name"
        chmod +x "$HOOKS_DIR/$hook_name"
        echo "✅ Installed $hook_name"
    else
        echo "❌ Hook $hook_name not found in $SOURCE_DIR"
    fi
}

install_hook "pre-commit"
install_hook "pre-push"

echo "Done. Gitleaks integration is now active."
