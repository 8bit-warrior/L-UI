#!/usr/bin/env python3
"""L-UI launcher for ordered source fragments."""
from __future__ import annotations

import __future__
from pathlib import Path

_ROOT = Path(__file__).resolve().parent
_PARTS = sorted((_ROOT / "src").glob("part_*.py"))
if not _PARTS:
    raise RuntimeError("L-UI source fragments are missing")

for _part in _PARTS:
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
