#!/usr/bin/env bash
set -euo pipefail

# Generate AI-powered changelog using OpenRouter
# Usage: ./scripts/generate-changelog.sh [TAG]
# If TAG is not provided, generates changelog for changes since the latest tag
#
# Configuration (via environment variables):
#   OPENROUTER_API_KEY    - Required. Your OpenRouter API key.
#   OPENROUTER_KEY        - Fallback for OPENROUTER_API_KEY.
#   OPENROUTER_MODEL      - Model ID (default: openai/gpt-4o-mini).
#                         Examples: anthropic/claude-sonnet-4,
#                                   anthropic/claude-opus-4,
#                                   deepseek/deepseek-chat,
#                                   openai/gpt-4o
#   OPENROUTER_TEMPERATURE - Sampling temperature (default: 0.3).
#   OPENROUTER_MAX_TOKENS  - Max tokens in response (default: 2000).

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

OPENROUTER_API_KEY="${OPENROUTER_API_KEY:-${OPENROUTER_KEY:-}}"
if [ -z "$OPENROUTER_API_KEY" ]; then
  echo "Error: OPENROUTER_API_KEY or OPENROUTER_KEY environment variable required" >&2
  exit 1
fi

OPENROUTER_MODEL="${OPENROUTER_MODEL:-openai/gpt-4o-mini}"
TEMPERATURE="${OPENROUTER_TEMPERATURE:-0.3}"
MAX_TOKENS="${OPENROUTER_MAX_TOKENS:-2000}"

echo "Using model: $OPENROUTER_MODEL" >&2

# Determine the range
# Usage:
#   ./generate-changelog.sh v0.0.13   → changelog for v0.0.12..v0.0.13
#   ./generate-changelog.sh           → changelog for latest-tag..HEAD (CI mode)
if [ $# -ge 1 ]; then
  CURRENT_REF="$1"
  PREVIOUS_TAG=$(git describe --tags --abbrev=0 "$CURRENT_REF^" 2>/dev/null || echo "")
else
  PREVIOUS_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
  CURRENT_REF="HEAD"
  if [ -z "$PREVIOUS_TAG" ]; then
    echo "Error: No tags found in repository" >&2
    exit 1
  fi
fi

if [ -n "$PREVIOUS_TAG" ]; then
  RANGE="${PREVIOUS_TAG}..${CURRENT_REF}"
  echo "Generating changelog from $PREVIOUS_TAG to $CURRENT_REF" >&2
else
  RANGE="$CURRENT_REF"
  echo "Generating changelog up to $CURRENT_REF (first release)" >&2
fi

# Collect commit messages
COMMITS=$(git log "$RANGE" --pretty=format:"%H%n%s%n%b%n---COMMIT_SEP---" 2>/dev/null || true)
if [ -z "$COMMITS" ]; then
  echo "Error: No commits found in range $RANGE" >&2
  exit 1
fi

# Collect file changes summary (not full diff to stay within token limits)
FILE_CHANGES=$(git diff --stat "$RANGE" 2>/dev/null || git diff-tree --no-commit-id --name-status -r "$CURRENT_REF" 2>/dev/null || true)

# Build the prompt
PROMPT=$(cat <<'PROMPT_EOF'
You are a technical writer generating release notes for a software project.

Analyze the provided git commits and file changes, then generate a well-structured
changelog in Markdown format. Follow these guidelines:

1. Start with a brief summary paragraph (2-3 sentences) describing the overall theme
2. Group changes into sections:
   - **New Features** - New capabilities, enhancements, additions
   - **Bug Fixes** - Fixes for reported or discovered issues
   - **Improvements** - Performance, reliability, or code quality improvements
   - **Documentation** - README, docs, comments updates
   - **Chores** - Build, CI/CD, dependency updates, refactoring
3. Each bullet should be concise but descriptive (one line)
4. Mention breaking changes prominently with a ⚠️ warning
5. Credit contributors when identifiable from commit authors
6. Use present tense, imperative mood (e.g., "Add feature" not "Added feature")

Format output as clean Markdown without any preamble or explanation.
PROMPT_EOF
)

# Build the user content with commits and file changes
USER_CONTENT="Git commits since last release:\n\n${COMMITS}\n\n"
if [ -n "$FILE_CHANGES" ]; then
  USER_CONTENT+="Files changed:\n\n${FILE_CHANGES}\n"
fi

# Truncate if too long (rough token estimate: ~4 chars per token)
MAX_CHARS=12000
if [ "${#USER_CONTENT}" -gt "$MAX_CHARS" ]; then
  USER_CONTENT="${USER_CONTENT:0:MAX_CHARS}\n\n[...truncated for length...]"
fi

# Call OpenRouter API
RESPONSE=$(curl -s -L -X POST "https://openrouter.ai/api/v1/chat/completions" \
  -H "Authorization: Bearer ${OPENROUTER_API_KEY}" \
  -H "Content-Type: application/json" \
  -d "$(jq -n \
    --arg prompt "$PROMPT" \
    --arg user "$USER_CONTENT" \
    --arg model "$OPENROUTER_MODEL" \
    --arg temp "$TEMPERATURE" \
    --arg tokens "$MAX_TOKENS" \
    '{
      model: $model,
      messages: [
        {role: "system", content: $prompt},
        {role: "user", content: $user}
      ],
      temperature: ($temp | tonumber),
      max_tokens: ($tokens | tonumber)
    }' \
  )" \
)

# Extract the changelog content
CHANGELOG=$(echo "$RESPONSE" | jq -r '.choices[0].message.content // empty' 2>/dev/null || true)

if [ -z "$CHANGELOG" ] || [ "$CHANGELOG" = "null" ]; then
  echo "Error: Failed to generate changelog from OpenRouter" >&2
  echo "API response: $RESPONSE" >&2
  exit 1
fi

echo "$CHANGELOG"
