#!/bin/sh

set -eu

usage() {
	cat <<'EOF'
Usage: scripts/benchmark_memory.sh BASELINE CANDIDATE ROOT

Compare mdsql task-query output and peak RSS without printing result contents.
EOF
}

if [ "$#" -ne 3 ]; then
	usage >&2
	exit 2
fi

baseline=$1
candidate=$2
root=$3

if [ ! -x "$baseline" ]; then
	echo "error: baseline is not executable" >&2
	exit 2
fi
if [ ! -x "$candidate" ]; then
	echo "error: candidate is not executable" >&2
	exit 2
fi
if [ ! -d "$root" ]; then
	echo "error: root is not a directory" >&2
	exit 2
fi

time_bin=${MDSQL_TIME_BIN:-/usr/bin/time}
if [ ! -x "$time_bin" ]; then
	echo "error: /usr/bin/time is unavailable" >&2
	exit 2
fi

case $(uname -s) in
	Darwin)
		time_style=darwin
		;;
	Linux)
		if ! "$time_bin" --version 2>&1 | grep -q 'GNU time'; then
			echo "error: GNU time is required on Linux" >&2
			exit 2
		fi
		time_style=gnu
		;;
	*)
		echo "error: supported platforms are macOS and Linux with GNU time" >&2
		exit 2
		;;
esac

if command -v shasum >/dev/null 2>&1; then
	hash_file() {
		shasum -a 256 "$1" | awk '{print $1}'
	}
elif command -v sha256sum >/dev/null 2>&1; then
	hash_file() {
		sha256sum "$1" | awk '{print $1}'
	}
else
	echo "error: shasum or sha256sum is required" >&2
	exit 2
fi

work_dir=$(mktemp -d "${TMPDIR:-/tmp}/mdsql-memory-benchmark.XXXXXX")
cleanup() {
	rm -rf "$work_dir"
}
trap cleanup EXIT HUP INT TERM

query="SELECT file, text FROM tasks WHERE state = 'todo'"

measure() {
	binary=$1
	stdout_file=$2
	stderr_file=$3
	time_file=$4
	exit_file=$5

	set +e
	if [ "$time_style" = darwin ]; then
		LC_ALL=C "$time_bin" -l -o "$time_file" \
			"$binary" --format=jsonl --max-rows=0 --no-cache "$query" "$root" \
			>"$stdout_file" 2>"$stderr_file"
	else
		LC_ALL=C "$time_bin" -v -o "$time_file" \
			"$binary" --format=jsonl --max-rows=0 --no-cache "$query" "$root" \
			>"$stdout_file" 2>"$stderr_file"
	fi
	rc=$?
	set -e
	printf '%s\n' "$rc" >"$exit_file"
}

rss_bytes() {
	if [ "$time_style" = darwin ]; then
		awk '/maximum resident set size/ {print $1; found = 1} END {if (!found) exit 1}' "$1"
	else
		awk -F ': *' '/Maximum resident set size \(kbytes\)/ {printf "%.0f\n", $2 * 1024; found = 1} END {if (!found) exit 1}' "$1"
	fi
}

report() {
	label=$1
	stdout_file=$2
	stderr_file=$3
	time_file=$4
	exit_file=$5

	rc=$(cat "$exit_file")
	rows=$(wc -l <"$stdout_file" | tr -d ' ')
	bytes=$(wc -c <"$stdout_file" | tr -d ' ')
	hash=$(hash_file "$stdout_file")
	rss=$(rss_bytes "$time_file") || {
		echo "error: could not read $label peak RSS" >&2
		exit 2
	}
	stderr_bytes=$(wc -c <"$stderr_file" | tr -d ' ')
	printf '%s: exit=%s rows=%s bytes=%s sha256=%s peak_rss_bytes=%s stderr_bytes=%s\n' \
		"$label" "$rc" "$rows" "$bytes" "$hash" "$rss" "$stderr_bytes"
}

measure "$baseline" "$work_dir/baseline.out" "$work_dir/baseline.err" \
	"$work_dir/baseline.time" "$work_dir/baseline.exit"
measure "$candidate" "$work_dir/candidate.out" "$work_dir/candidate.err" \
	"$work_dir/candidate.time" "$work_dir/candidate.exit"

report baseline "$work_dir/baseline.out" "$work_dir/baseline.err" \
	"$work_dir/baseline.time" "$work_dir/baseline.exit"
report candidate "$work_dir/candidate.out" "$work_dir/candidate.err" \
	"$work_dir/candidate.time" "$work_dir/candidate.exit"

baseline_rc=$(cat "$work_dir/baseline.exit")
candidate_rc=$(cat "$work_dir/candidate.exit")
if [ "$baseline_rc" -ne 0 ] || [ "$candidate_rc" -ne 0 ]; then
	echo "FAIL: at least one query failed; stderr contents were suppressed" >&2
	exit 1
fi

baseline_hash=$(hash_file "$work_dir/baseline.out")
candidate_hash=$(hash_file "$work_dir/candidate.out")
if [ "$baseline_hash" != "$candidate_hash" ]; then
	echo "FAIL: baseline and candidate stdout differ" >&2
	exit 1
fi

candidate_rss=$(rss_bytes "$work_dir/candidate.time")
rss_limit=$((100 * 1024 * 1024))
if [ "$candidate_rss" -gt "$rss_limit" ]; then
	echo "FAIL: candidate peak RSS exceeds 100 MiB" >&2
	exit 1
fi

echo "PASS: stdout matches and candidate peak RSS is at most 100 MiB"
