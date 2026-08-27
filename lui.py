#!/usr/bin/env python3
"""L-UI launcher.

The implementation is split into ordered source fragments under ``src/`` so the
project stays readable while still behaving as one panel-less terminal script.
All fragments execute in this module's single global namespace.
"""
from __future__ import annotations

import __future__
from pathlib import Path

_ROOT = Path(__file__).resolve().parent
_PARTS = [
    _ROOT / "src" / "lui_part_01.py",
    _ROOT / "src" / "lui_part_02.py",
    _ROOT / "src" / "lui_part_03.py",
    _ROOT / "src" / "lui_part_04.py",
    _ROOT / "src" / "lui_part_05.py",
]

for _part in _PARTS:
    if not _part.is_file():
        raise RuntimeError(f"L-UI source fragment missing: {_part}")
    _source = _part.read_text(encoding="utf-8")
    exec(
        compile(
            _source,
            str(_part),
            "exec",
            flags=__future__.annotations.compiler_flag,
            dont_inherit=True,
        ),
        globals(),
        globals(),
    )
