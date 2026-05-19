#!/usr/bin/env python3
# Python helper invoked from Go parity tests.
# Reads a JSON command from stdin, writes a JSON result to stdout.
#
# Schema (input):
#   {"op": "mfcc",         "wav": "/path"}
#   {"op": "mfcc_data",    "samples": [...], "sample_rate": 16000}
#   {"op": "dtw_path",     "mfcc1": "/path", "mfcc2": "/path", "delta": 3000}
#   {"op": "dtw_exact",    "mfcc1_inline": [[...]], "mfcc2_inline": [[...]]}
#   {"op": "vad",          "energy": [...], "min_nonspeech_length": 3, "log_energy_threshold": 0.7, "extend_before": 0, "extend_after": 0}
#   {"op": "wav_samples",  "wav": "/path"}
#   {"op": "audiofile_mfcc", "wav": "/path"}
#
# Schema (output): JSON object varies by op; on error: {"error": "..."}.
#
# Designed to be a long-lived process: in "server" mode, repeatedly
# reads newline-delimited JSON commands and emits newline-delimited results.
# In "single" mode (default), processes one command and exits.

import json
import os
import sys
import time
import traceback

import numpy as np

from aeneas.audiofile import AudioFile


def _read_mfcc_text(path):
    return np.loadtxt(path)


def op_mfcc(req):
    import aeneas.cmfcc.cmfcc as cmfcc
    af = AudioFile(req["wav"])
    af.read_samples_from_file()
    t0 = time.perf_counter()
    out = cmfcc.compute_from_data(
        af.audio_samples,
        af.audio_sample_rate,
        int(req.get("filter_bank_size", 40)),
        int(req.get("mfcc_size", 13)),
        int(req.get("fft_order", 512)),
        float(req.get("lower_frequency", 133.3333)),
        float(req.get("upper_frequency", 6855.4976)),
        float(req.get("emphasis_factor", 0.97)),
        float(req.get("window_length", 0.025)),
        float(req.get("window_shift", 0.010)),
    )[0].transpose()
    t1 = time.perf_counter()
    return {
        "shape": list(out.shape),
        "matrix": out.tolist(),
        "elapsed_seconds": t1 - t0,
        "num_samples": int(len(af.audio_samples)),
        "sample_rate": int(af.audio_sample_rate),
    }


def op_mfcc_data(req):
    import aeneas.cmfcc.cmfcc as cmfcc
    samples = np.array(req["samples"], dtype=np.float64)
    out = cmfcc.compute_from_data(
        samples,
        int(req["sample_rate"]),
        int(req.get("filter_bank_size", 40)),
        int(req.get("mfcc_size", 13)),
        int(req.get("fft_order", 512)),
        float(req.get("lower_frequency", 133.3333)),
        float(req.get("upper_frequency", 6855.4976)),
        float(req.get("emphasis_factor", 0.97)),
        float(req.get("window_length", 0.025)),
        float(req.get("window_shift", 0.010)),
    )[0].transpose()
    return {"shape": list(out.shape), "matrix": out.tolist()}


def op_dtw_path(req):
    # Match how aeneas.dtw.DTWStripe drives the C extension: discard the
    # first MFCC row before calling compute_best_path. The Go port does the
    # same in dtw.ComputeCostMatrix, so the paths are directly comparable.
    import aeneas.cdtw.cdtw as cdtw
    mfcc1 = _read_mfcc_text(req["mfcc1"])
    mfcc2 = _read_mfcc_text(req["mfcc2"])
    if req.get("discard_first_row", True):
        m1 = mfcc1[1:, :]
        m2 = mfcc2[1:, :]
    else:
        m1, m2 = mfcc1, mfcc2
    _, m = m2.shape
    delta = int(req.get("delta", 3000))
    if delta > m:
        delta = m
    t0 = time.perf_counter()
    path = cdtw.compute_best_path(m1, m2, delta)
    t1 = time.perf_counter()
    return {
        "n": int(m1.shape[1]),
        "m": int(m),
        "delta": int(delta),
        "len": len(path),
        "path": [[int(p[0]), int(p[1])] for p in path],
        "elapsed_seconds": t1 - t0,
    }


def op_dtw_exact(req):
    # Exact (full) DTW via the pure-Python fallback in aeneas/dtw.py.
    # We reproduce the cost-matrix + accumulation + backtrace directly so we
    # don't need to construct full AudioFileMFCC objects.
    mfcc1 = np.array(req["mfcc1_inline"], dtype=np.float64)
    mfcc2 = np.array(req["mfcc2_inline"], dtype=np.float64)
    # Discard the first MFCC row to match aeneas behaviour.
    m1 = mfcc1[1:]
    m2 = mfcc2[1:]
    n = m1.shape[1]
    m = m2.shape[1]
    n1 = np.linalg.norm(m1, axis=0)
    n2 = np.linalg.norm(m2, axis=0)
    cost = 1.0 - (m1.T @ m2) / np.outer(n1, n2)
    # Accumulate
    acc = cost.copy()
    for j in range(1, m):
        acc[0, j] += acc[0, j - 1]
    for i in range(1, n):
        acc[i, 0] += acc[i - 1, 0]
    for i in range(1, n):
        for j in range(1, m):
            acc[i, j] += min(acc[i - 1, j], acc[i, j - 1], acc[i - 1, j - 1])
    # Backtrace
    path = [(n - 1, m - 1)]
    i, j = n - 1, m - 1
    while i > 0 or j > 0:
        if i == 0:
            j -= 1
        elif j == 0:
            i -= 1
        else:
            a, b, c = acc[i - 1, j], acc[i, j - 1], acc[i - 1, j - 1]
            if a <= b and a <= c:
                i -= 1
            elif b <= c:
                j -= 1
            else:
                i -= 1
                j -= 1
        path.append((i, j))
    path.reverse()
    return {"n": n, "m": m, "len": len(path), "path": [[int(p[0]), int(p[1])] for p in path]}


def op_vad(req):
    from aeneas.vad import VAD
    from aeneas.runtimeconfiguration import RuntimeConfiguration
    rconf = RuntimeConfiguration()
    vad = VAD(rconf=rconf)
    energy = np.array(req["energy"], dtype=np.float64)
    t0 = time.perf_counter()
    mask = vad.run_vad(
        energy,
        log_energy_threshold=float(req["log_energy_threshold"]),
        min_nonspeech_length=int(req["min_nonspeech_length"]),
        extend_before=int(req.get("extend_before", 0)),
        extend_after=int(req.get("extend_after", 0)),
    )
    t1 = time.perf_counter()
    return {
        "mask": [bool(b) for b in mask],
        "elapsed_seconds": t1 - t0,
    }


def op_wav_samples(req):
    # Read raw samples bypassing aeneas's automatic FFmpeg resampling.
    # aeneas's own audiofile module re-exports scipy.io.wavfile under the
    # alias `scipywavread`, so we go through that to get untouched samples
    # at the source rate / depth, matching what the Go audio.WAVFile reader
    # emits.
    from aeneas.audiofile import scipywavread
    sr, data = scipywavread(req["wav"])
    if data.ndim > 1:
        data = data[:, 0]
    if data.dtype == np.int16:
        samples = data.astype(np.float64) / 32768.0
    elif data.dtype == np.int32:
        samples = data.astype(np.float64) / 2147483648.0
    elif data.dtype == np.uint8:
        samples = (data.astype(np.float64) - 128.0) / 128.0
    elif data.dtype == np.float32 or data.dtype == np.float64:
        samples = data.astype(np.float64)
    else:
        raise ValueError(f"unsupported dtype {data.dtype}")
    n = int(req.get("max_samples", min(len(samples), 64)))
    return {
        "sample_rate": int(sr),
        "num_samples": int(len(samples)),
        "first_samples": [float(x) for x in samples[:n]],
    }


def op_audiofile_mfcc(req):
    # Build an AudioFileMFCC (the wrapper used by aeneas) for cross-check.
    from aeneas.audiofilemfcc import AudioFileMFCC
    from aeneas.runtimeconfiguration import RuntimeConfiguration
    rconf = RuntimeConfiguration()
    afm = AudioFileMFCC(file_path=req["wav"], rconf=rconf)
    return {
        "all_length": int(afm.all_length),
        "head_begin": int(afm.head_begin),
        "head_end": int(afm.head_end),
        "middle_begin": int(afm.middle_begin),
        "middle_end": int(afm.middle_end),
        "tail_begin": int(afm.tail_begin),
        "tail_end": int(afm.tail_end),
        "all_mfcc_shape": list(afm.all_mfcc.shape),
    }


HANDLERS = {
    "mfcc": op_mfcc,
    "mfcc_data": op_mfcc_data,
    "dtw_path": op_dtw_path,
    "dtw_exact": op_dtw_exact,
    "vad": op_vad,
    "wav_samples": op_wav_samples,
    "audiofile_mfcc": op_audiofile_mfcc,
}


def handle(req):
    op = req.get("op")
    h = HANDLERS.get(op)
    if h is None:
        return {"error": f"unknown op: {op}"}
    try:
        return h(req)
    except Exception as exc:
        return {"error": f"{exc.__class__.__name__}: {exc}", "trace": traceback.format_exc()}


def main():
    mode = os.environ.get("PYHELPER_MODE", "single")
    if mode == "server":
        for line in sys.stdin:
            line = line.strip()
            if not line:
                continue
            try:
                req = json.loads(line)
            except json.JSONDecodeError as exc:
                sys.stdout.write(json.dumps({"error": f"bad JSON: {exc}"}) + "\n")
                sys.stdout.flush()
                continue
            res = handle(req)
            sys.stdout.write(json.dumps(res) + "\n")
            sys.stdout.flush()
    else:
        req = json.loads(sys.stdin.read())
        res = handle(req)
        json.dump(res, sys.stdout)


if __name__ == "__main__":
    main()
