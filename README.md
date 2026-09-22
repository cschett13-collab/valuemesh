# valuemesh

**Value-maximization as a protocol for an agent fleet.** Stop counting tokens.
Measure what your agents actually *produced*, make them accountable **together**,
and change every agent at once through one shared directive.

Foundation models are commoditized; the value is in the **system** around them —
orchestration, governance, accountability. `valuemesh` is that governance layer for
a fleet of AI agents, and it runs **local** (your models, your box, your data).

## The idea

- **Outcomes over consumption.** A token is a cost, not a result. Agents report the
  *outcome* they produced (work done, time saved) — not how much they burned.
- **Ask the agents.** The hub reflects on each agent's real logs with a local model
  and grades value vs. waste — bluntly, and grounded in what actually happened.
- **One fleet, together.** Every report lands in a shared ledger; the board ranks
  the fleet by value and flags the idle "bad stuff."
- **Change them everywhere.** There is one shared **directive** file. Every agent
  reads it. Edit it once and the whole fleet's contract changes — systems over models.

## Use

```sh
# an agent self-reports an outcome (the beacon — one line, any language)
valuemesh report --agent crawler --did "indexed 4k pages, 0 dupes" --saved-min 90

# ask the hub to grade an agent from its real logs (local model, no fabrication)
valuemesh ask crawler --log /var/log/crawler.log

# the collective view + (re)write the shared directive every agent reads
valuemesh board

# print the shared value-maximization contract
valuemesh directive
```

Shared state lives at `~/.valuemesh/{ledger.jsonl, directive.md}`.

## Configure (env)

| var | default | meaning |
|-----|---------|---------|
| `LITELLM_BASE` | `http://127.0.0.1:4000` | OpenAI-compatible endpoint for `ask` |
| `VALUEMESH_MODEL` | `fast` | model used to grade agents |
| `MODELGATE_STATS` | `http://127.0.0.1:4060/api/stats` | optional cost/revenue signal |
| `AGENT_LOG_DIR` | `~/Desktop/ai-agents/logs` | where agent logs/status live |
| `VALUEMESH_DIR` | `~/.valuemesh` | shared ledger + directive |

Any OpenAI-compatible local server works (llama.cpp/vLLM/Ollama/LM Studio via a
router). No cloud required; nothing leaves your machine.

## Adopt in 1 line

Have each agent end a unit of work with:

```sh
valuemesh report --agent "$NAME" --did "<what you accomplished>"
```

That's it — the agent is now part of the fleet's value accounting, and reads the
same directive as everyone else.

## License

MIT — see [LICENSE](LICENSE).
