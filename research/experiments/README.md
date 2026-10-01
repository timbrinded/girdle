# Decision-engine experiments

These September 2026 probes compare a Jev-compatible `/v1/systemone` server on decisions a coding harness makes. The recorded results are historical observations; model aliases, prices and backend setup may have changed. The [research index](../README.md) connects them to the wider measurements.

The root scripts use Python's standard library for HTTP requests and analysis. Run them from this directory. Hosted probes call a provider and can incur costs; they use `TYPESAFE_API_KEY` when set. The original recorded responses are retained as `jev_*.txt` and `kev08_*.txt`.

## Hosted Jev probes

Export the TypeSafe key in the launching shell, then select the endpoint and model explicitly:

```bash
python3 harness_decisions_eval.py https://api.typesafe.ai jev-1.13.0
python3 risk_decomposed.py        https://api.typesafe.ai jev-1.13.0
python3 choice_distributions.py   https://api.typesafe.ai jev-1.13.0
python3 score_scales.py           https://api.typesafe.ai jev-1.13.0
```

The scripts examine status/choice distributions, decomposed action risks and score scales. These commands generate new provider responses; they do not replay the saved text files.

## Local Kev method

The original local runs used Kev-0.8B on an M3 Pro with MLX and bf16. The historical setup below runs **from a separate clone of `github.com/jaredpalmer/kev`**, using its dependency manifest and serving extra:

```bash
uv sync --extra serve
uv run --extra serve python -m kev.serve --run jaredpalmer/kev-0.8b --port 8009
```

With that server running, return to this experiment directory and point a probe at it:

```bash
python3 harness_decisions_eval.py http://127.0.0.1:8009 kev-latest
```

Consult the backend's own setup for its supported platform and current interface. Girdle does not bundle that backend.

## Follow-up methods

| Directory | Purpose | Required local inputs |
| --- | --- | --- |
| [fanout](fanout/README.md) | Replay checkpoint states with larger question banks | Benchmark logs, generated replay JSON, warm scale checkouts and snapshot dumps |
| [astgrep](astgrep/README.md) | Structural lookup and shell-tripwire prototypes | ast-grep, benchmark logs/checkouts and fanout intermediates |
| [hard](hard/README.md) | Mine upstream fixes and construct tasks | Full-history upstream clones |

The original benchmark corpus and generated intermediates are not bundled. These guides explain how the methods used those inputs. Use the [benchmark guide](../../docs/benchmarks.md) to generate new runs, with its platform and isolation limits. Removed features remain here as research machinery rather than current Girdle options.
