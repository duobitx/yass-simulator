# Makefile wrapper for go-task (https://taskfile.dev)
#
# Behavior:
#   - `make` runs `task` (default task from Taskfile.yml)
#   - `make <target>` runs `task <target>`
#   - `make <t1> <t2>` runs `task <t1>` then `task <t2>`
#   - Pass extra flags to task via ARGS, e.g.: `make build ARGS="--yes --force"`
#
# Requirement 1: Check if `task` is installed; otherwise, error.
# Requirement 2: Run `task` with all arguments.

SHELL := /bin/sh
.DEFAULT_GOAL := default

.PHONY: check_task default

check_task:
	@command -v task >/dev/null 2>&1 || { \
		echo "Error: 'task' command not found in PATH." >&2; \
		echo "Please install go-task: https://taskfile.dev/#/installation" >&2; \
		exit 1; \
	}

# When no goal is provided, run `task` as-is (uses Taskfile default)
default: check_task
	@task $(ARGS)

# Forward any provided goal to `task <goal>`
Makefile: ;
%: check_task
	@task $@ $(ARGS)
