# Decision-engine experiments

Probes a Jev-compatible `/v1/systemone` server on the decisions a harness makes.

```bash
# local Kev (from a clone of github.com/jaredpalmer/kev)
uv sync --extra serve
uv run --extra serve python -m kev.serve --run jaredpalmer/kev-0.8b --port 8009

python3 harness_decisions_eval.py http://127.0.0.1:8009 kev-latest
python3 risk_decomposed.py        http://127.0.0.1:8009 kev-latest
python3 choice_distributions.py   http://127.0.0.1:8009 kev-latest   # group mass, expected cost, ranking
python3 score_scales.py           http://127.0.0.1:8009 kev-latest   # damage, complexity, urgency scales
```

Hosted Jev: the scripts send `Authorization: Bearer $TYPESAFE_API_KEY` when that variable is set.

```bash
export TYPESAFE_API_KEY=...   # defined in ~/.zshrc
python3 harness_decisions_eval.py https://api.typesafe.ai jev-latest
```

`jev_*.txt` are the recorded hosted-Jev runs (jev-1.13, 25 Sep 2026).
`kev08_*.txt` are the recorded Kev-0.8B runs (M3 Pro, MLX, bf16).
