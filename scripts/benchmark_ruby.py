#!/usr/bin/env python3
"""Run small, output-checked RGo/MRI end-to-end benchmarks."""

import os
from pathlib import Path
import shutil
import statistics
import subprocess
import sys
import time


ROOT = Path(__file__).resolve().parent.parent
RGO = Path(os.environ.get("RGO_BIN", ROOT / "rgo")).resolve()
MRI = os.environ.get("RGO_MRI") or shutil.which("ruby")
REPEATS = int(os.environ.get("RGO_BENCH_REPEATS", "9"))
STARTUP_REPEATS = int(os.environ.get("RGO_BENCH_STARTUP_REPEATS", "21"))
BENCH_NICE = int(os.environ.get("RGO_BENCH_NICE", "15"))
BENCH_CPU = os.environ.get("RGO_BENCH_CPU", "").strip()


def benchmark_cpu_set():
    if not BENCH_CPU:
        return None
    cpus = set()
    for item in BENCH_CPU.split(","):
        item = item.strip()
        if not item:
            continue
        if "-" in item:
            start, end = (int(part) for part in item.split("-", 1))
            cpus.update(range(start, end + 1))
        else:
            cpus.add(int(item))
    return cpus or None


BENCH_CPU_SET = benchmark_cpu_set()


def setup_benchmark_child():
    if BENCH_CPU_SET is not None and hasattr(os, "sched_setaffinity"):
        os.sched_setaffinity(0, BENCH_CPU_SET)
    if BENCH_NICE:
        os.nice(BENCH_NICE)


def checked_output(argv, env):
    return subprocess.run(
        argv,
        cwd=ROOT,
        env=env,
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        preexec_fn=setup_benchmark_child,
    ).stdout


def run_once(argv, env):
    started = time.perf_counter()
    pid = os.fork()
    if pid == 0:
        setup_benchmark_child()
        null_fd = os.open(os.devnull, os.O_WRONLY)
        os.dup2(null_fd, 1)
        os.dup2(null_fd, 2)
        os.chdir(ROOT)
        os.execve(argv[0], argv, env)
    _, status, usage = os.wait4(pid, 0)
    elapsed = time.perf_counter() - started
    if status != 0:
        raise RuntimeError(f"command failed ({status}): {argv}")
    return elapsed, usage.ru_maxrss


def measure(argv, env, repeats):
    run_once(argv, env)
    samples = [run_once(argv, env) for _ in range(repeats)]
    times = [sample[0] for sample in samples]
    rss = [sample[1] for sample in samples]
    return statistics.median(times), min(times), max(times), statistics.median(rss)


def main():
    if not RGO.is_file():
        print("missing ./rgo; run `make build` first", file=sys.stderr)
        return 2
    if not MRI:
        print("MRI Ruby not found; set RGO_MRI=/path/to/ruby", file=sys.stderr)
        return 2

    rgo_env = os.environ.copy()
    mri_env = os.environ.copy()
    cases = [
        ("startup", ["-e", "nil"], STARTUP_REPEATS),
        ("arith", [str(ROOT / "bench/ruby/arith.rb")], REPEATS),
        ("dispatch", [str(ROOT / "bench/ruby/dispatch.rb")], REPEATS),
        ("blocks", [str(ROOT / "bench/ruby/blocks.rb")], REPEATS),
        ("collections", [str(ROOT / "bench/ruby/collections.rb")], REPEATS),
        ("strings", [str(ROOT / "bench/ruby/strings.rb")], REPEATS),
    ]

    print("case,engine,median_ms,min_ms,max_ms,median_rss_kb,rgo_over_mri")
    for name, args, repeats in cases:
        rgo_argv = [str(RGO)] + args
        mri_argv = [str(Path(MRI)), "--disable-gems"] + args
        if checked_output(rgo_argv, rgo_env) != checked_output(mri_argv, mri_env):
            raise RuntimeError(f"{name}: RGo and MRI output differ")
        rgo = measure(rgo_argv, rgo_env, repeats)
        mri = measure(mri_argv, mri_env, repeats)
        ratio = rgo[0] / mri[0] if mri[0] > 0 else float("inf")
        print(f"{name},RGO,{rgo[0]*1000:.3f},{rgo[1]*1000:.3f},{rgo[2]*1000:.3f},{rgo[3]:.0f},{ratio:.3f}")
        print(f"{name},MRI,{mri[0]*1000:.3f},{mri[1]*1000:.3f},{mri[2]*1000:.3f},{mri[3]:.0f},1.000")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
