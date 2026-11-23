## Instructions for agents

### Follow Go proverbs
https://go-proverbs.github.io/
Don't communicate by sharing memory, share memory by communicating.
Concurrency is not parallelism.
Channels orchestrate; mutexes serialize.
The bigger the interface, the weaker the abstraction.
Make the zero value useful.
interface{} says nothing.
Gofmt's style is no one's favorite, yet gofmt is everyone's favorite.
A little copying is better than a little dependency.
Syscall must always be guarded with build tags.
Cgo must always be guarded with build tags.
Cgo is not Go.
With the unsafe package there are no guarantees.
Clear is better than clever.
Reflection is never clear.
Errors are values.
Don't just check errors, handle them gracefully.
Design the architecture, name the components, document the details.
Documentation is for users.
Don't panic.


###  Practical
- Include a tree of the files in readme per the directory.
- In micropython, no authored function may exceed 35 physical lines, including comments, blank lines, and callbacks. oxlint enforces this. do not include python classes until requested
- No authored source file may exceed 200 lines. Aim below 150 when the responsibilities divide naturally. The source-file check includes Astro, CSS, JavaScript, TypeScript, scripts, and tests.
- Comments should be used to explain things that can’t be expressed in code, like the rationale for why a certain decision was made.
- Current state in README.md
- As your context gets filled up, record changes made to CHANGELOG.md. add takeaways, theoretical considerations or future questions to CONTEXT.md. Record in this format: initial state, goal state, obstacle, scale.
- Human will write to AGENT.md and DESIGN.md
- Prefer mutually exclusive, collectively exhaustive domain boundaries. Be precise. Names should be borrowed from physics, cs, engineering or cognitive science.
- Compare existing tools by features, complexity, community size and date published

### Typescript and python
- Use `pnpm add` for TypeScript dependencies and `uv add` for Python dependencies.
- Use oxlint and Prettier for TypeScript and Ruff for future Python research.
- Run Python through `uv run`, never bare `python` or `python3`.
- Observability: Track model-fitting runs in Weights & Biases; use Cloudflare observability for runtime operations.
- Place run instructions in `README.md` after each significant update
- Do not remove user comments in any scripts
- Put representative visual output in `examples`; keep scratch output untracked.
- Never commit plaintext secrets. Use committed `.env.op` references after the
- Pause after a stage and update the README with minimal edits.
- Do not automatically commit or deploy after a stage.

Verification
- uv sync + Python tests pass with slim deps.
- pnpm -C torrent lint / typecheck / test green
- under 500 ms network latency
- Orchestrator falls back to Terminal.app windows (tmux isn't installed here) — verify that path.


- automatically run commands, do not just supply commands to the user
- when you start input scan, start a ghostty terminal
