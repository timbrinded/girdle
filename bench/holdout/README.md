# Held-out tasks

These tasks measure Girdle at milestones. Nothing is designed or tuned from their results, and they aren't run during development (decision 0016). Development uses `bench/tasks`, `bench/scale` and `bench/hard`.

Two groups:

- **From the hard suite:** cmp-equate-comparable, ex-find, ex-let-variables, gm-cjk-runes, gm-403-406, mi-running-stats, mux-invalid-query, pf-func, tm-quoted-dots, tm-base-ints and tm-inline-dotted. The fast flow was calibrated on them once, before the split (decision 0015). No feature has been tuned on them.
- **Fresh:** sv-*, gv-*, hz-*, gj-*, gt-* and gb-*, from six repositories Girdle's development never touched: semver, go-version, go-humanize, gjson, go-toml and gobwas/glob. An agent with no knowledge of Girdle wrote their statements from each fix's commit message and hidden tests, and a second one checked each statement against its tests. Girdle has never run on them.

Run them with:

```bash
bench/bench.sh -w -s holdout -a "girdle-fast girdle" -r 3
```
